package connector

import (
	"context"
	"fmt"
	"time"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	apidefaults "github.com/gravitational/teleport/api/defaults"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/conductorone/baton-teleport/pkg/client"
)

const (
	auditEventFeedID = "teleport_audit_events"

	// Teleport resource lifecycle event type strings passed to SearchEvents.
	// Teleport supports EventOrderAscending natively, so C1's ordering
	// expectation is met without any result-reversal logic.
	//
	// IMPORTANT – naming convention (verified against Teleport v18 live API):
	//   Teleport is inconsistent between resource types:
	//   - User/app/db events use bare present-tense: "user.create", "app.delete"
	//   - Role/lock events use past-tense:           "role.created", "lock.deleted"
	//   Always verify with SearchEvents(nil filter) before adding new types.
	//
	// IMPORTANT – user events use SEPARATE types for create vs update
	//   (verified against Teleport v18 live API 2026-02-27):
	//   - CreateUser() → fires "user.create" (Go type: *events.UserCreate)
	//   - UpdateUser() → fires "user.update" (Go type: *events.UserUpdate)
	//   Both carry the complete Roles list in their payload.
	userCreateEventType = "user.create"
	userUpdateEventType = "user.update"

	// Teleport fires separate event types for role create vs update
	// (verified against live API 2026-02-27: "role.updated" fires with
	// code T9002I and Go type *events.RoleUpdate).
	roleCreateEventType = "role.created"
	roleUpdateEventType = "role.updated"

	// Access request events (verified against Teleport v18 live API 2026-02-27):
	//   All event types below use the SAME Go type (*events.AccessRequestCreate).
	//   User and Roles fields are EMPTY — only RequestID and RequestState are
	//   set. We must look up the original request by ID to recover them.
	//
	//   - "access_request.review" fires when a reviewer approves or denies.
	//   - "access_request.update" fires when the request state transitions
	//     (e.g. APPROVED).
	//   - "access_request.expire" fires when a time-limited request expires
	//     and the temporary roles are revoked.
	//
	//   "access_request.create" is intentionally excluded: it fires when a user
	//   submits a request (PENDING). At that point no access has been granted
	//   yet, so there is nothing new for C1 to discover via resync.
	accessRequestReviewEventType = "access_request.review"
	accessRequestUpdateEventType = "access_request.update"
	accessRequestExpireEventType = "access_request.expire"

	// Lock events use past-tense strings (verified against live API).
	// lock.created fires when a user is suspended via a Teleport lock.
	// lock.deleted fires when the lock is removed (user re-enabled).
	//
	// Only user targets are relevant: Get() on a user surfaces IsLocked
	// as STATUS_DISABLED/STATUS_ENABLED. Role targets are ignored because
	// the role resource has no lock/status field — Get() returns nothing new.
	lockCreateEventType = "lock.created"
	lockDeleteEventType = "lock.deleted"
)

// resourceChangeEventTypes is the list of Teleport audit events that signal a
// resource was created or modified.
//
// DELETE events (user.delete, role.deleted, app.delete, db.delete,
// access_request.delete) are intentionally excluded:
// ResourceChangeEvent triggers C1 to call Get() for the resource, but a
// deleted resource returns "not found" which C1 cannot distinguish from an
// error. Deletions are reconciled during the next full sync instead.
//
// access_request.create is also excluded: the request is just PENDING at that
// point and no access has been granted — there is nothing new to resync.
//
// lock.deleted IS included (unlike other delete events) because it deletes
// the *lock*, not the user. The user still exists and Get() returns them
// with IsLocked=false → STATUS_ENABLED.
//
// app.create, app.update, db.create, db.update are intentionally excluded:
// List() uses Metadata.Revision as the resource ID for apps and databases,
// but audit events only carry the resource name — C1 cannot correlate the
// event to an existing resource. Until the ID strategy is resolved, these
// resource types are excluded from incremental sync and reconciled during
// the next full sync instead.
var resourceChangeEventTypes = []string{
	userCreateEventType, userUpdateEventType,
	roleCreateEventType, roleUpdateEventType,
	accessRequestReviewEventType, accessRequestUpdateEventType, accessRequestExpireEventType,
	lockCreateEventType, lockDeleteEventType,
}

type auditEventFeed struct {
	client *client.TeleportClient
}

func (e *auditEventFeed) EventFeedMetadata(_ context.Context) *v2.EventFeedMetadata {
	return &v2.EventFeedMetadata{
		Id: auditEventFeedID,
		SupportedEventTypes: []v2.EventType{
			v2.EventType_EVENT_TYPE_RESOURCE_CHANGE,
			v2.EventType_EVENT_TYPE_CREATE_GRANT,
			v2.EventType_EVENT_TYPE_CREATE_REVOKE,
		},
	}
}

func (e *auditEventFeed) ListEvents(
	ctx context.Context,
	earliestEvent *timestamppb.Timestamp,
	pToken *pagination.StreamToken,
) ([]*v2.Event, *pagination.StreamState, annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	cursor, err := unmarshalEventPageCursor(pToken, earliestEvent)
	if err != nil {
		return nil, nil, nil, err
	}

	from, err := time.Parse(time.RFC3339Nano, cursor.StartAt)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("baton-teleport: failed to parse audit start_at: %w", err)
	}
	to := time.Now().UTC()

	// Teleport supports EventOrderAscending directly — no need to reverse,
	// unlike providers that only expose descending order.
	auditEvents, lastKey, err := e.client.SearchEvents(
		ctx,
		from,
		to,
		apidefaults.Namespace,
		resourceChangeEventTypes,
		eventsPageSize,
		types.EventOrderAscending,
		cursor.LastKey,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("baton-teleport: failed to search audit events: %w", err)
	}

	l.Debug("fetched audit events",
		zap.Int("count", len(auditEvents)),
		zap.String("last_key", lastKey),
	)

	var result []*v2.Event
	for _, auditEvent := range auditEvents {
		// Access request state-change events (review, update, expire) carry
		// empty User and Roles fields. We must resolve the original request by
		// ID before converting, so this case is handled here rather than in the
		// pure convertAuditEvent function.
		if derived, eventTime, ok, err := e.tryConvertAccessRequestStateChange(ctx, auditEvent); err != nil {
			return nil, nil, nil, err
		} else if ok {
			result = append(result, derived...)
			cursor.updateLatestEvent(eventTime)
			continue
		}

		derived, eventTime := convertAuditEvent(auditEvent)
		result = append(result, derived...)
		cursor.updateLatestEvent(eventTime)
	}

	if lastKey == "" {
		cursor.prepareNextSync()
	} else {
		cursor.LastKey = lastKey
	}

	marshalledCursor, err := cursor.marshal()
	if err != nil {
		return nil, nil, nil, err
	}

	return result, &pagination.StreamState{
		Cursor:  marshalledCursor,
		HasMore: lastKey != "",
	}, nil, nil
}

// convertAuditEvent maps a single Teleport AuditEvent to one or more baton
// Events and returns the event timestamp for cursor tracking.
//
// A single Teleport event may produce multiple baton events: for example,
// user.create emits a ResourceChangeEvent for the user AND a CreateGrantEvent
// for each role initially assigned, because we know the grants definitively.
//
// Teleport event behaviour (verified against v18 live API 2026-02-27):
//   - User events use SEPARATE types: CreateUser() → "user.create",
//     UpdateUser() → "user.update". Both carry the complete Roles list.
//   - Role events also use SEPARATE types: "role.created" → *events.RoleCreate,
//     "role.updated" (code T9002I) → *events.RoleUpdate.
func convertAuditEvent(auditEvent events.AuditEvent) ([]*v2.Event, time.Time) {
	switch e := auditEvent.(type) {
	// --- User create (fires "user.create" → *events.UserCreate):
	//     emit a ResourceChangeEvent + a CreateGrantEvent per role.
	//     The Roles slice is the complete post-operation role list.
	case *events.UserCreate:
		return convertUserEvent(e.GetID(), e.GetTime(), e.Name, e.Roles)

	// --- User update (fires "user.update" → *events.UserUpdate):
	//     Teleport v18 fires a SEPARATE event type for user modifications
	//     (verified against live API 2026-02-27). Same payload shape as
	//     UserCreate — carries the complete new role list.
	case *events.UserUpdate:
		return convertUserEvent(e.GetID(), e.GetTime(), e.Name, e.Roles)

	// --- Role create (fires "role.created" → *events.RoleCreate):
	case *events.RoleCreate:
		return singleResourceChange(e.GetID(), e.GetTime(), roleResourceType.Id, e.Name)
	// --- Role update (fires "role.updated" → *events.RoleUpdate):
	//     Verified against live API 2026-02-27: role modifications fire a
	//     separate "role.updated" event with code T9002I.
	case *events.RoleUpdate:
		return singleResourceChange(e.GetID(), e.GetTime(), roleResourceType.Id, e.Name)
	// --- Locks ---
	// Only user targets are handled: Get() surfaces IsLocked as STATUS_DISABLED/
	// STATUS_ENABLED. Role targets are ignored (role Get() has no lock status).
	case *events.LockCreate:
		return convertLockEvent(e.GetID(), e.GetTime(), e.Lock.Target)
	case *events.LockDelete:
		return convertLockEvent(e.GetID(), e.GetTime(), e.Lock.Target)
	}

	return nil, time.Time{}
}

// convertUserEvent handles both *events.UserCreate and *events.UserUpdate.
// Both carry the complete role list in their Roles field. We emit a
// ResourceChangeEvent for the user plus a CreateGrantEvent per role.
func convertUserEvent(id string, t time.Time, userName string, roles []string) ([]*v2.Event, time.Time) {
	if userName == "" {
		return nil, time.Time{}
	}
	var out []*v2.Event
	out = append(out, makeResourceChangeEvent(id, t, userResourceType.Id, userName))
	for _, roleName := range roles {
		if roleName == "" {
			continue
		}
		out = append(out, makeCreateGrantEvent(
			fmt.Sprintf("%s:role:%s", id, roleName),
			t,
			roleName,
			userName,
		))
	}
	return out, t
}

// singleResourceChange returns a one-element slice containing a
// ResourceChangeEvent, or nil if the resource name is empty.
func singleResourceChange(id string, t time.Time, resourceType, resourceName string) ([]*v2.Event, time.Time) {
	if resourceName == "" {
		return nil, time.Time{}
	}
	return []*v2.Event{makeResourceChangeEvent(id, t, resourceType, resourceName)}, t
}

// makeResourceChangeEvent builds a v2.Event wrapping a ResourceChangeEvent.
func makeResourceChangeEvent(id string, t time.Time, resourceType, resourceName string) *v2.Event {
	return &v2.Event{
		Id:         id,
		OccurredAt: timestamppb.New(t),
		Event: &v2.Event_ResourceChangeEvent{
			ResourceChangeEvent: &v2.ResourceChangeEvent{
				ResourceId: &v2.ResourceId{
					ResourceType: resourceType,
					Resource:     resourceName,
				},
			},
		},
	}
}

// roleMembershipEntitlementAndPrincipal builds the shared entitlement and
// principal used by both grant and revoke event constructors.
func roleMembershipEntitlementAndPrincipal(roleName, userName string) (*v2.Entitlement, *v2.Resource) {
	roleResource := &v2.Resource{
		Id: &v2.ResourceId{
			ResourceType: roleResourceType.Id,
			Resource:     roleName,
		},
		DisplayName: roleName,
	}

	return ent.NewAssignmentEntitlement(roleResource, roleMembership), &v2.Resource{
		Id: &v2.ResourceId{
			ResourceType: userResourceType.Id,
			Resource:     userName,
		},
		DisplayName: userName,
	}
}

// makeCreateGrantEvent builds a v2.Event wrapping a CreateGrantEvent that
// signals a role membership was assigned to a user.
func makeCreateGrantEvent(id string, t time.Time, roleName, userName string) *v2.Event {
	entitlement, principal := roleMembershipEntitlementAndPrincipal(roleName, userName)
	return &v2.Event{
		Id:         id,
		OccurredAt: timestamppb.New(t),
		Event: &v2.Event_CreateGrantEvent{
			CreateGrantEvent: &v2.CreateGrantEvent{
				Entitlement: entitlement,
				Principal:   principal,
			},
		},
	}
}

// makeCreateRevokeEvent builds a v2.Event wrapping a CreateRevokeEvent that
// signals a role membership was removed from a user.
func makeCreateRevokeEvent(id string, t time.Time, roleName, userName string) *v2.Event {
	entitlement, principal := roleMembershipEntitlementAndPrincipal(roleName, userName)
	return &v2.Event{
		Id:         id,
		OccurredAt: timestamppb.New(t),
		Event: &v2.Event_CreateRevokeEvent{
			CreateRevokeEvent: &v2.CreateRevokeEvent{
				Entitlement: entitlement,
				Principal:   principal,
			},
		},
	}
}

// convertLockEvent emits a ResourceChangeEvent for the user targeted by a
// Teleport lock (create or delete). C1 calls Get() on the user, which
// surfaces IsLocked as STATUS_DISABLED (locked) or STATUS_ENABLED (unlocked).
//
// Role targets are ignored: the role resource has no lock/status field, so
// a ResourceChangeEvent would trigger a Get() that discovers nothing new.
// Other targets (node, login, MFA device, etc.) are also ignored.
func convertLockEvent(id string, t time.Time, target types.LockTarget) ([]*v2.Event, time.Time) {
	if target.User == "" {
		return nil, time.Time{}
	}
	return []*v2.Event{makeResourceChangeEvent(id, t, userResourceType.Id, target.User)}, t
}

// tryConvertAccessRequestStateChange handles access request events where the
// User/Roles fields are empty and we need an API lookup to resolve them.
// This covers "access_request.review", "access_request.update", and
// "access_request.expire" — all three use *events.AccessRequestCreate but
// only carry RequestID and RequestState (no User or Roles).
//
// Behaviour by state:
//   - APPROVED: emit CreateGrantEvent per role (user gained temporary access).
//   - EXPIRED:  emit CreateRevokeEvent per role (temporary access removed).
//   - DENIED:   nothing to emit (no access change occurred).
//
// We do NOT emit ResourceChangeEvent for access requests because the user's
// Get() only returns profile data (name, email, status) which hasn't changed.
// Role grants are a separate concern and are communicated via grant/revoke events.
//
// Returns (events, time, true, nil) if the event was handled, or
// (nil, zero, false, nil) if it should fall through to convertAuditEvent.
// Returns a non-nil error when the API lookup fails so the caller can retry.
func (e *auditEventFeed) tryConvertAccessRequestStateChange(
	ctx context.Context,
	auditEvent events.AuditEvent,
) ([]*v2.Event, time.Time, bool, error) {
	arc, ok := auditEvent.(*events.AccessRequestCreate)
	if !ok {
		return nil, time.Time{}, false, nil
	}

	// Only intercept state-change events (empty User signals this is a review,
	// update, or expire — not a submission). Submission events have User
	// populated and are handled by convertAuditEvent.
	if arc.User != "" {
		return nil, time.Time{}, false, nil
	}

	t := arc.GetTime()

	// Must have a RequestID to look up the original request.
	if arc.RequestID == "" {
		return nil, t, true, nil
	}

	l := ctxzap.Extract(ctx)

	if e.client == nil {
		l.Debug("cannot resolve access request state change: no client available",
			zap.String("request_id", arc.RequestID),
		)
		return nil, t, true, nil
	}

	// Look up the original access request to recover user and roles.
	requests, err := e.client.GetAccessRequests(ctx, types.AccessRequestFilter{
		ID: arc.RequestID,
	})
	if err != nil {
		return nil, time.Time{}, true, fmt.Errorf("baton-teleport: failed to resolve access request %s: %w", arc.RequestID, err)
	}
	if len(requests) == 0 {
		l.Debug("access request not found (may have been deleted or expired)",
			zap.String("request_id", arc.RequestID),
			zap.String("state", arc.RequestState),
		)
		return nil, t, true, nil
	}

	req := requests[0]
	userName := req.GetUser()
	roles := req.GetRoles()

	if userName == "" {
		return nil, t, true, nil
	}

	var out []*v2.Event

	switch arc.RequestState {
	case "APPROVED":
		for _, roleName := range roles {
			if roleName == "" {
				continue
			}
			out = append(out, makeCreateGrantEvent(
				fmt.Sprintf("%s:role:%s", arc.GetID(), roleName),
				t,
				roleName,
				userName,
			))
		}
	case "EXPIRED":
		for _, roleName := range roles {
			if roleName == "" {
				continue
			}
			out = append(out, makeCreateRevokeEvent(
				fmt.Sprintf("%s:role:%s", arc.GetID(), roleName),
				t,
				roleName,
				userName,
			))
		}
	default:
		// DENIED, PENDING, etc. — no access change occurred, nothing to emit.
	}

	l.Debug("resolved access request state change",
		zap.String("request_id", arc.RequestID),
		zap.String("state", arc.RequestState),
		zap.String("user", userName),
		zap.Strings("roles", roles),
		zap.Int("events_emitted", len(out)),
	)

	return out, t, true, nil
}

func newAuditEventFeed(c *client.TeleportClient) *auditEventFeed {
	return &auditEventFeed{client: c}
}
