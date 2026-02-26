package connector

import (
	"context"
	"fmt"
	"testing"
	"time"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/stretchr/testify/require"
)

// helper: assert the first (and only) event is a ResourceChangeEvent with
// the expected resource type and name.
func requireSingleResourceChange(t *testing.T, evts []*v2.Event, ts time.Time, wantType, wantName string) {
	t.Helper()
	require.Len(t, evts, 1)
	rc := evts[0].GetResourceChangeEvent()
	require.NotNil(t, rc, "expected ResourceChangeEvent")
	require.Equal(t, wantType, rc.ResourceId.ResourceType)
	require.Equal(t, wantName, rc.ResourceId.Resource)
	if !ts.IsZero() {
		require.Equal(t, ts.Unix(), evts[0].OccurredAt.AsTime().Unix())
	}
}

// --- UserCreate ---

func TestConvertAuditEvent_UserCreate_NoRoles(t *testing.T) {
	eventTime := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	e := &events.UserCreate{
		Metadata:         events.Metadata{ID: "uc-1", Time: eventTime},
		ResourceMetadata: events.ResourceMetadata{Name: "alice"},
	}
	evts, ts := convertAuditEvent(e)
	requireSingleResourceChange(t, evts, eventTime, userResourceType.Id, "alice")
	require.Equal(t, eventTime.Unix(), ts.Unix())
}

func TestConvertAuditEvent_UserCreate_WithRoles(t *testing.T) {
	eventTime := time.Date(2024, 5, 2, 10, 0, 0, 0, time.UTC)
	e := &events.UserCreate{
		Metadata:         events.Metadata{ID: "uc-2", Time: eventTime},
		ResourceMetadata: events.ResourceMetadata{Name: "bob"},
		Roles:            []string{"reviewer", "access"},
	}
	evts, ts := convertAuditEvent(e)
	// 1 ResourceChangeEvent + 2 CreateGrantEvents
	require.Len(t, evts, 3)
	require.Equal(t, eventTime.Unix(), ts.Unix())

	// First event: ResourceChangeEvent for the user
	rc := evts[0].GetResourceChangeEvent()
	require.NotNil(t, rc)
	require.Equal(t, userResourceType.Id, rc.ResourceId.ResourceType)
	require.Equal(t, "bob", rc.ResourceId.Resource)

	// Remaining events: CreateGrantEvent for each role
	roleNames := make(map[string]bool)
	for _, ev := range evts[1:] {
		cg := ev.GetCreateGrantEvent()
		require.NotNil(t, cg, "expected CreateGrantEvent")
		require.Equal(t, roleResourceType.Id, cg.Entitlement.Resource.Id.ResourceType)
		require.Equal(t, roleMembership, cg.Entitlement.Slug)
		require.Equal(t, userResourceType.Id, cg.Principal.Id.ResourceType)
		require.Equal(t, "bob", cg.Principal.Id.Resource)
		roleNames[cg.Entitlement.Resource.Id.Resource] = true
		// Event ID must be unique per role
		require.Contains(t, ev.Id, "uc-2")
	}
	require.True(t, roleNames["reviewer"])
	require.True(t, roleNames["access"])
}

func TestConvertAuditEvent_UserCreate_EmptyName(t *testing.T) {
	e := &events.UserCreate{
		Metadata:         events.Metadata{ID: "uc-empty"},
		ResourceMetadata: events.ResourceMetadata{Name: ""},
	}
	evts, ts := convertAuditEvent(e)
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

func TestConvertAuditEvent_UserCreate_SkipsEmptyRoleName(t *testing.T) {
	e := &events.UserCreate{
		Metadata:         events.Metadata{ID: "uc-3"},
		ResourceMetadata: events.ResourceMetadata{Name: "carol"},
		Roles:            []string{"", "access", ""},
	}
	evts, _ := convertAuditEvent(e)
	// 1 ResourceChangeEvent + 1 CreateGrantEvent (empty roles skipped)
	require.Len(t, evts, 2)
	require.NotNil(t, evts[0].GetResourceChangeEvent())
	require.NotNil(t, evts[1].GetCreateGrantEvent())
	require.Equal(t, "access", evts[1].GetCreateGrantEvent().Entitlement.Resource.Id.Resource)
}

// --- UserUpdate (fires "user.update" → *events.UserUpdate) ---
// Verified against Teleport v18 live API (2026-02-27): UpdateUser() fires
// "user.update" as a SEPARATE event type from "user.create". Both carry the
// complete Roles list.

func TestConvertAuditEvent_UserUpdate_WithRoles(t *testing.T) {
	eventTime := time.Date(2024, 5, 3, 0, 0, 0, 0, time.UTC)
	e := &events.UserUpdate{
		Metadata:         events.Metadata{ID: "uu-1", Time: eventTime},
		ResourceMetadata: events.ResourceMetadata{Name: "dave"},
		Roles:            []string{"reviewer", "auditor"},
	}
	evts, ts := convertAuditEvent(e)
	// 1 ResourceChangeEvent + 2 CreateGrantEvents (one per role).
	require.Len(t, evts, 3)
	require.Equal(t, eventTime.Unix(), ts.Unix())

	rc := evts[0].GetResourceChangeEvent()
	require.NotNil(t, rc)
	require.Equal(t, userResourceType.Id, rc.ResourceId.ResourceType)
	require.Equal(t, "dave", rc.ResourceId.Resource)

	roleNames := make(map[string]bool)
	for _, ev := range evts[1:] {
		cg := ev.GetCreateGrantEvent()
		require.NotNil(t, cg, "expected CreateGrantEvent")
		roleNames[cg.Entitlement.Resource.Id.Resource] = true
	}
	require.True(t, roleNames["reviewer"])
	require.True(t, roleNames["auditor"])
}

func TestConvertAuditEvent_UserUpdate_NoRoles(t *testing.T) {
	eventTime := time.Date(2024, 5, 3, 0, 0, 0, 0, time.UTC)
	e := &events.UserUpdate{
		Metadata:         events.Metadata{ID: "uu-noroles", Time: eventTime},
		ResourceMetadata: events.ResourceMetadata{Name: "dave"},
	}
	evts, ts := convertAuditEvent(e)
	requireSingleResourceChange(t, evts, eventTime, userResourceType.Id, "dave")
	require.Equal(t, eventTime.Unix(), ts.Unix())
}

func TestConvertAuditEvent_UserUpdate_EmptyName(t *testing.T) {
	e := &events.UserUpdate{
		Metadata:         events.Metadata{ID: "uu-empty"},
		ResourceMetadata: events.ResourceMetadata{Name: ""},
	}
	evts, ts := convertAuditEvent(e)
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

func TestConvertAuditEvent_UserUpdate_SkipsEmptyRoleName(t *testing.T) {
	e := &events.UserUpdate{
		Metadata:         events.Metadata{ID: "uu-skip"},
		ResourceMetadata: events.ResourceMetadata{Name: "dave"},
		Roles:            []string{"", "access", ""},
	}
	evts, _ := convertAuditEvent(e)
	// 1 ResourceChangeEvent + 1 CreateGrantEvent (empty roles skipped)
	require.Len(t, evts, 2)
	require.NotNil(t, evts[0].GetResourceChangeEvent())
	require.NotNil(t, evts[1].GetCreateGrantEvent())
	require.Equal(t, "access", evts[1].GetCreateGrantEvent().Entitlement.Resource.Id.Resource)
}

// --- UserDelete (not handled — deletions reconciled by full sync) ---

func TestConvertAuditEvent_UserDelete_IsNotHandled(t *testing.T) {
	e := &events.UserDelete{
		Metadata:         events.Metadata{ID: "ud-1"},
		ResourceMetadata: events.ResourceMetadata{Name: "eve"},
	}
	evts, ts := convertAuditEvent(e)
	// Delete events are intentionally not handled: ResourceChangeEvent would
	// trigger Get() which returns "not found" for deleted resources.
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

// --- Role CRUD ---

func TestConvertAuditEvent_RoleCreate(t *testing.T) {
	e := &events.RoleCreate{
		Metadata:         events.Metadata{ID: "rc-1"},
		ResourceMetadata: events.ResourceMetadata{Name: "reviewer"},
	}
	evts, _ := convertAuditEvent(e)
	requireSingleResourceChange(t, evts, time.Time{}, roleResourceType.Id, "reviewer")
}

func TestConvertAuditEvent_RoleUpdate(t *testing.T) {
	// Verified against live API 2026-02-27: role modifications fire
	// "role.updated" (code T9002I) → *events.RoleUpdate.
	e := &events.RoleUpdate{
		Metadata:         events.Metadata{ID: "ru-1"},
		ResourceMetadata: events.ResourceMetadata{Name: "editor"},
	}
	evts, _ := convertAuditEvent(e)
	requireSingleResourceChange(t, evts, time.Time{}, roleResourceType.Id, "editor")
}

func TestConvertAuditEvent_RoleDelete_IsNotHandled(t *testing.T) {
	e := &events.RoleDelete{
		Metadata:         events.Metadata{ID: "rd-1"},
		ResourceMetadata: events.ResourceMetadata{Name: "old-role"},
	}
	evts, ts := convertAuditEvent(e)
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

// --- App CRUD ---

func TestConvertAuditEvent_AppCreate_IsNotHandled(t *testing.T) {
	e := &events.AppCreate{
		Metadata:         events.Metadata{ID: "ac-1"},
		ResourceMetadata: events.ResourceMetadata{Name: "my-app"},
	}
	evts, ts := convertAuditEvent(e)
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

func TestConvertAuditEvent_AppUpdate_IsNotHandled(t *testing.T) {
	e := &events.AppUpdate{
		Metadata:         events.Metadata{ID: "au-1"},
		ResourceMetadata: events.ResourceMetadata{Name: "my-app"},
	}
	evts, ts := convertAuditEvent(e)
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

func TestConvertAuditEvent_AppDelete_IsNotHandled(t *testing.T) {
	e := &events.AppDelete{
		Metadata:         events.Metadata{ID: "ad-1"},
		ResourceMetadata: events.ResourceMetadata{Name: "old-app"},
	}
	evts, ts := convertAuditEvent(e)
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

// --- Database CRUD ---

func TestConvertAuditEvent_DatabaseCreate_IsNotHandled(t *testing.T) {
	e := &events.DatabaseCreate{
		Metadata:         events.Metadata{ID: "dc-1"},
		ResourceMetadata: events.ResourceMetadata{Name: "prod-db"},
	}
	evts, ts := convertAuditEvent(e)
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

func TestConvertAuditEvent_DatabaseUpdate_IsNotHandled(t *testing.T) {
	e := &events.DatabaseUpdate{
		Metadata:         events.Metadata{ID: "du-1"},
		ResourceMetadata: events.ResourceMetadata{Name: "prod-db"},
	}
	evts, ts := convertAuditEvent(e)
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

func TestConvertAuditEvent_DatabaseDelete_IsNotHandled(t *testing.T) {
	e := &events.DatabaseDelete{
		Metadata:         events.Metadata{ID: "dd-1"},
		ResourceMetadata: events.ResourceMetadata{Name: "old-db"},
	}
	evts, ts := convertAuditEvent(e)
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

// --- Unknown / empty ---

func TestConvertAuditEvent_UnknownType(t *testing.T) {
	evts, ts := convertAuditEvent(&events.SessionStart{})
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

// --- Event ID uniqueness for derived events ---

func TestConvertAuditEvent_DerivedEventIDsAreUnique(t *testing.T) {
	e := &events.UserCreate{
		Metadata:         events.Metadata{ID: "base-id"},
		ResourceMetadata: events.ResourceMetadata{Name: "frank"},
		Roles:            []string{"r1", "r2", "r3"},
	}
	evts, _ := convertAuditEvent(e)
	require.Len(t, evts, 4)

	seen := make(map[string]bool)
	for _, ev := range evts {
		require.False(t, seen[ev.Id], "duplicate event ID: %s", ev.Id)
		seen[ev.Id] = true
	}
}

// --- Grant event entitlement format matches full sync ---

func TestConvertAuditEvent_GrantEntitlementIDMatchesFullSync(t *testing.T) {
	e := &events.UserCreate{
		Metadata:         events.Metadata{ID: "uc-match"},
		ResourceMetadata: events.ResourceMetadata{Name: "grace"},
		Roles:            []string{"reviewer"},
	}
	evts, _ := convertAuditEvent(e)
	require.Len(t, evts, 2)

	grantEvt := evts[1].GetCreateGrantEvent()
	require.NotNil(t, grantEvt)

	// Entitlement ID format must match roles.go: "role:{roleName}:member"
	expectedID := fmt.Sprintf("%s:%s:%s", roleResourceType.Id, "reviewer", roleMembership)
	require.Equal(t, expectedID, grantEvt.Entitlement.Id)
}

// --- Access Requests ---
//
// All access request events use the SAME Go type (*events.AccessRequestCreate).
//   - "access_request.review" fires on reviewer action (APPROVED/DENIED).
//   - "access_request.update" fires on state transition (APPROVED/DENIED).
//   - "access_request.expire" fires when a time-limited request expires.
//
// "access_request.create" (submission, PENDING) is NOT subscribed to because
// no access has been granted yet — there is nothing new for C1 to discover.
//
// Review/update/expire events have empty User/Roles fields and are handled
// exclusively by tryConvertAccessRequestStateChange (API lookup required).
// convertAuditEvent has no AccessRequestCreate case — all access request
// events are intercepted before convertAuditEvent is called.

func TestConvertAuditEvent_AccessRequestCreate_NotHandled(t *testing.T) {
	// *events.AccessRequestCreate is not in the convertAuditEvent switch.
	// All access request events are handled by tryConvertAccessRequestStateChange.
	e := &events.AccessRequestCreate{
		Metadata:     events.Metadata{ID: "ar-1"},
		UserMetadata: events.UserMetadata{User: "alice"},
		Roles:        []string{"reviewer"},
		RequestState: "APPROVED",
	}
	evts, ts := convertAuditEvent(e)
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

// --- tryConvertAccessRequestStateChange ---
//
// These tests cover the state-change handler's branching logic. The full
// approval flow (API lookup → CreateGrantEvents) and expire flow (API
// lookup → CreateRevokeEvents) require a live client and are verified via
// the live integration tests in cmd/test-events.
//
// Behaviour by state:
//   - APPROVED: emit CreateGrantEvent per role
//   - EXPIRED:  emit CreateRevokeEvent per role
//   - DENIED:   nothing emitted (no access change)
//
// This handler covers "access_request.review", "access_request.update",
// and "access_request.expire" — all three use *events.AccessRequestCreate
// with empty User/Roles fields.

func TestTryConvertAccessRequestStateChange_NonAccessRequestEvent(t *testing.T) {
	feed := newAuditEventFeed(nil)
	e := &events.UserCreate{
		Metadata:         events.Metadata{ID: "uc-1"},
		ResourceMetadata: events.ResourceMetadata{Name: "alice"},
	}
	_, _, ok, err := feed.tryConvertAccessRequestStateChange(context.Background(), e)
	require.NoError(t, err)
	require.False(t, ok, "non-AccessRequestCreate event should return ok=false")
}

func TestTryConvertAccessRequestStateChange_SubmissionNotIntercepted(t *testing.T) {
	// Submission events (User is populated) should not be intercepted —
	// they fall through to convertAuditEvent.
	feed := newAuditEventFeed(nil)
	e := &events.AccessRequestCreate{
		Metadata:     events.Metadata{ID: "ar-submit"},
		UserMetadata: events.UserMetadata{User: "alice"},
		Roles:        []string{"editor"},
		RequestState: "PENDING",
	}
	_, _, ok, err := feed.tryConvertAccessRequestStateChange(context.Background(), e)
	require.NoError(t, err)
	require.False(t, ok, "submission event (User populated) should return ok=false")
}

func TestTryConvertAccessRequestStateChange_EmptyRequestID(t *testing.T) {
	// State-change event with no RequestID — handled (true) but nothing emitted.
	// The event time should still be returned so the cursor advances.
	eventTime := time.Date(2024, 7, 1, 10, 0, 0, 0, time.UTC)
	feed := newAuditEventFeed(nil)
	e := &events.AccessRequestCreate{
		Metadata:     events.Metadata{ID: "ar-no-id", Time: eventTime},
		UserMetadata: events.UserMetadata{User: ""},
		RequestState: "APPROVED",
		RequestID:    "",
	}
	evts, t2, ok, err := feed.tryConvertAccessRequestStateChange(context.Background(), e)
	require.NoError(t, err)
	require.True(t, ok, "state-change event should return ok=true (handled)")
	require.Nil(t, evts, "no events emitted without RequestID")
	require.Equal(t, eventTime.Unix(), t2.Unix(), "event time should be returned for cursor advancement")
}

func TestTryConvertAccessRequestStateChange_NilClientGracefulFailure(t *testing.T) {
	// State-change event with RequestID but nil client — handled gracefully,
	// no events emitted (logs a debug message in production).
	feed := newAuditEventFeed(nil)
	e := &events.AccessRequestCreate{
		Metadata:     events.Metadata{ID: "ar-nil-client", Time: time.Now()},
		UserMetadata: events.UserMetadata{User: ""},
		RequestState: "APPROVED",
		RequestID:    "some-request-uuid",
	}
	ctx := context.Background()
	evts, _, ok, err := feed.tryConvertAccessRequestStateChange(ctx, e)
	require.NoError(t, err)
	require.True(t, ok, "state-change event should be handled")
	require.Nil(t, evts, "no events when client is nil")
}

func TestTryConvertAccessRequestStateChange_UpdateEvent(t *testing.T) {
	// "access_request.update" fires when the request state transitions.
	// Same Go type as review — empty User, requires lookup.
	// Nil client → no API call → no events, but no error.
	feed := newAuditEventFeed(nil)
	e := &events.AccessRequestCreate{
		Metadata:     events.Metadata{ID: "ar-update", Time: time.Now()},
		UserMetadata: events.UserMetadata{User: ""},
		RequestState: "APPROVED",
		RequestID:    "update-request-uuid",
	}
	ctx := context.Background()
	evts, _, ok, err := feed.tryConvertAccessRequestStateChange(ctx, e)
	require.NoError(t, err)
	require.True(t, ok, "update event should be handled")
	require.Nil(t, evts, "no events when client is nil")
}

func TestTryConvertAccessRequestStateChange_ExpireEvent(t *testing.T) {
	// "access_request.expire" fires when a time-limited request expires.
	// Same Go type — empty User, minimal payload, requires lookup.
	// Nil client → no API call → no events, but no error.
	feed := newAuditEventFeed(nil)
	e := &events.AccessRequestCreate{
		Metadata:     events.Metadata{ID: "ar-expire", Time: time.Now()},
		UserMetadata: events.UserMetadata{User: ""},
		RequestState: "",
		RequestID:    "expire-request-uuid",
	}
	ctx := context.Background()
	evts, _, ok, err := feed.tryConvertAccessRequestStateChange(ctx, e)
	require.NoError(t, err)
	require.True(t, ok, "expire event should be handled")
	require.Nil(t, evts, "no events when client is nil")
}

func TestConvertAuditEvent_AccessRequestDelete_IsNotHandled(t *testing.T) {
	e := &events.AccessRequestDelete{
		Metadata:     events.Metadata{ID: "ard-1"},
		UserMetadata: events.UserMetadata{User: "carol"},
	}
	evts, ts := convertAuditEvent(e)
	// Delete events are intentionally not handled.
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

// --- Locks ---

func TestConvertAuditEvent_LockCreate_UserTarget(t *testing.T) {
	eventTime := time.Date(2024, 6, 4, 10, 0, 0, 0, time.UTC)
	e := &events.LockCreate{
		Metadata: events.Metadata{ID: "lc-1", Time: eventTime},
		Lock: events.LockMetadata{
			Target: types.LockTarget{User: "dave"},
		},
	}
	evts, ts := convertAuditEvent(e)
	requireSingleResourceChange(t, evts, eventTime, userResourceType.Id, "dave")
	require.Equal(t, eventTime.Unix(), ts.Unix())
}

func TestConvertAuditEvent_LockCreate_RoleTarget_Ignored(t *testing.T) {
	// Role targets are ignored: role Get() has no lock/status field.
	e := &events.LockCreate{
		Metadata: events.Metadata{ID: "lc-2"},
		Lock: events.LockMetadata{
			Target: types.LockTarget{Role: "auditor"},
		},
	}
	evts, ts := convertAuditEvent(e)
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

func TestConvertAuditEvent_LockCreate_UserAndRoleTarget(t *testing.T) {
	// Only the user target is emitted; role target is ignored.
	eventTime := time.Date(2024, 6, 6, 10, 0, 0, 0, time.UTC)
	e := &events.LockCreate{
		Metadata: events.Metadata{ID: "lc-3", Time: eventTime},
		Lock: events.LockMetadata{
			Target: types.LockTarget{User: "eve", Role: "reviewer"},
		},
	}
	evts, ts := convertAuditEvent(e)
	requireSingleResourceChange(t, evts, eventTime, userResourceType.Id, "eve")
	require.Equal(t, eventTime.Unix(), ts.Unix())
}

func TestConvertAuditEvent_LockCreate_UnknownTarget(t *testing.T) {
	// Lock targets a login — we don't model logins.
	e := &events.LockCreate{
		Metadata: events.Metadata{ID: "lc-node"},
		Lock: events.LockMetadata{
			Target: types.LockTarget{Login: "root"},
		},
	}
	evts, ts := convertAuditEvent(e)
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

// --- Lock delete (user re-enabled) ---

func TestConvertAuditEvent_LockDelete_UserTarget(t *testing.T) {
	// lock.deleted removes the lock → user flips from IsLocked=true to false.
	// Get() returns STATUS_ENABLED.
	eventTime := time.Date(2024, 6, 7, 10, 0, 0, 0, time.UTC)
	e := &events.LockDelete{
		Metadata: events.Metadata{ID: "ld-1", Time: eventTime},
		Lock: events.LockMetadata{
			Target: types.LockTarget{User: "frank"},
		},
	}
	evts, ts := convertAuditEvent(e)
	requireSingleResourceChange(t, evts, eventTime, userResourceType.Id, "frank")
	require.Equal(t, eventTime.Unix(), ts.Unix())
}

func TestConvertAuditEvent_LockDelete_RoleTarget_Ignored(t *testing.T) {
	e := &events.LockDelete{
		Metadata: events.Metadata{ID: "ld-2"},
		Lock: events.LockMetadata{
			Target: types.LockTarget{Role: "editor"},
		},
	}
	evts, ts := convertAuditEvent(e)
	require.Nil(t, evts)
	require.True(t, ts.IsZero())
}

// --- Metadata ---

func TestAuditEventFeedMetadata(t *testing.T) {
	feed := newAuditEventFeed(nil)
	meta := feed.EventFeedMetadata(context.Background())
	require.Equal(t, auditEventFeedID, meta.Id)
	require.Len(t, meta.SupportedEventTypes, 3)

	typeSet := make(map[v2.EventType]bool)
	for _, et := range meta.SupportedEventTypes {
		typeSet[et] = true
	}
	require.True(t, typeSet[v2.EventType_EVENT_TYPE_RESOURCE_CHANGE])
	require.True(t, typeSet[v2.EventType_EVENT_TYPE_CREATE_GRANT])
	require.True(t, typeSet[v2.EventType_EVENT_TYPE_CREATE_REVOKE])
}
