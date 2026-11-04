package connector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/conductorone/baton-sdk/pkg/actions"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/google/uuid"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"google.golang.org/protobuf/types/known/structpb"

	config "github.com/conductorone/baton-sdk/pb/c1/config/v1"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-teleport/pkg/client"
)

const (
	lockUserActionName   = "lock_user"
	unlockUserActionName = "unlock_user"

	argUser     = "user"
	argDuration = "duration"
	argMessage  = "message"

	defaultLockMessage = "This user has been locked by ConductorOne."
)

// lockAPI is the subset of the Teleport client used by the lock actions.
type lockAPI interface {
	GetUser(ctx context.Context, name string, withSecrets bool) (types.User, error)
	UpsertLock(ctx context.Context, lock types.Lock) error
	DeleteLock(ctx context.Context, name string) error
	ListLocks(ctx context.Context, limit int, startKey string, filter *types.LockFilter) ([]types.Lock, string, error)
}

var _ connectorbuilder.GlobalActionProvider = (*Connector)(nil)
var _ lockAPI = (*client.TeleportClient)(nil)

type lockActions struct {
	api lockAPI
	now func() time.Time
}

// GlobalActions registers the connector's global actions.
func (d *Connector) GlobalActions(ctx context.Context, registry actions.ActionRegistry) error {
	// A nil client (e.g. capabilities generation) must not be wrapped in a non-nil interface.
	var api lockAPI
	if d.client != nil {
		api = d.client
	}
	la := &lockActions{api: api, now: time.Now}
	if err := registry.Register(ctx, lockUserSchema(), la.lockUser); err != nil {
		return fmt.Errorf("baton-teleport: failed to register %s action: %w", lockUserActionName, err)
	}
	if err := registry.Register(ctx, unlockUserSchema(), la.unlockUser); err != nil {
		return fmt.Errorf("baton-teleport: failed to register %s action: %w", unlockUserActionName, err)
	}
	return nil
}

func stringArgField(name, displayName, description, placeholder string, required bool) *config.Field {
	return config.Field_builder{
		Name:        name,
		DisplayName: displayName,
		Description: description,
		Placeholder: placeholder,
		IsRequired:  required,
		StringField: &config.StringField{},
	}.Build()
}

func userArgField() *config.Field {
	return stringArgField(argUser, "User", "The Teleport username of the user.", "jane-doe", true)
}

func lockUserSchema() *v2.BatonActionSchema {
	return &v2.BatonActionSchema{
		Name:        lockUserActionName,
		DisplayName: "Lock Teleport user",
		Description: "Locks a Teleport user so that new requests are rejected and active sessions are terminated. " +
			"The lock stays in force until it expires or is removed with unlock_user.",
		ActionType: []v2.ActionType{v2.ActionType_ACTION_TYPE_ACCOUNT_DISABLE},
		Arguments: []*config.Field{
			userArgField(),
			stringArgField(argDuration, "Duration",
				"Optional. How long the lock stays in force, as a positive Go duration such as 30m, 24h or 168h. Days are not supported. If omitted the lock does not expire.",
				"24h", false),
			stringArgField(argMessage, "Message", "Optional. Message shown to the locked-out user.", defaultLockMessage, false),
		},
		ReturnTypes: []*config.Field{
			stringArgField("lock_name", "Lock name", "Name of the created Teleport lock.", "", false),
			stringArgField("expires", "Expires", "RFC3339 expiry of the lock, empty if it does not expire.", "", false),
		},
	}
}

func unlockUserSchema() *v2.BatonActionSchema {
	return &v2.BatonActionSchema{
		Name:        unlockUserActionName,
		DisplayName: "Unlock Teleport user",
		Description: "Removes all Teleport locks that target exactly this user. Locks on roles, devices or other targets are left untouched.",
		ActionType:  []v2.ActionType{v2.ActionType_ACTION_TYPE_ACCOUNT_ENABLE},
		Arguments:   []*config.Field{userArgField()},
		ReturnTypes: []*config.Field{
			stringArgField("deleted_locks", "Deleted locks", "Number of locks removed.", "", false),
		},
	}
}

// parseLockDuration parses an optional duration argument. It returns 0 when absent.
func parseLockDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("baton-teleport: invalid %s %q (use a Go duration such as 30m or 24h): %w", argDuration, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("baton-teleport: %s must be positive, got %q", argDuration, raw)
	}
	return d, nil
}

// durationArg reads the optional duration. Accepts a string, or a number of seconds.
func durationArg(args *structpb.Struct) (time.Duration, error) {
	if v, ok := args.GetFields()[argDuration]; ok {
		if _, isNum := v.GetKind().(*structpb.Value_NumberValue); isNum {
			secs := v.GetNumberValue()
			if secs <= 0 || secs > float64(1<<40) {
				return 0, fmt.Errorf("baton-teleport: %s must be a positive number of seconds, got %v", argDuration, secs)
			}
			return time.Duration(secs * float64(time.Second)), nil
		}
	}
	raw, _ := actions.GetStringArg(args, argDuration)
	return parseLockDuration(raw)
}

func (l *lockActions) requireUser(ctx context.Context, args *structpb.Struct) (string, error) {
	if l.api == nil {
		return "", fmt.Errorf("baton-teleport: teleport client is not configured")
	}
	user, err := actions.RequireStringArg(args, argUser)
	if err != nil {
		return "", fmt.Errorf("baton-teleport: %w", err)
	}
	user = strings.TrimSpace(user)
	if user == "" {
		return "", fmt.Errorf("baton-teleport: %s is required", argUser)
	}
	if _, err := l.api.GetUser(ctx, user, false); err != nil {
		if trace.IsNotFound(err) {
			return "", fmt.Errorf("baton-teleport: user %q not found: %w", user, err)
		}
		return "", fmt.Errorf("baton-teleport: failed to get user %q: %w", user, err)
	}
	return user, nil
}

func (l *lockActions) lockUser(ctx context.Context, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
	user, err := l.requireUser(ctx, args)
	if err != nil {
		return nil, nil, err
	}
	d, err := durationArg(args)
	if err != nil {
		return nil, nil, err
	}
	message, _ := actions.GetStringArg(args, argMessage)
	if strings.TrimSpace(message) == "" {
		message = defaultLockMessage
	}

	spec := types.LockSpecV2{
		Target:  types.LockTarget{User: user},
		Message: message,
	}
	var expires string
	if d > 0 {
		t := l.now().UTC().Add(d)
		spec.Expires = &t
		expires = t.Format(time.RFC3339)
	}

	// A fresh UUID name (as `tctl lock` does) means upsert never overwrites an existing lock.
	name := uuid.NewString()
	lock, err := types.NewLock(name, spec)
	if err != nil {
		return nil, nil, fmt.Errorf("baton-teleport: failed to build lock for user %q: %w", user, err)
	}
	if err := l.api.UpsertLock(ctx, lock); err != nil {
		return nil, nil, fmt.Errorf("baton-teleport: failed to lock user %q: %w", user, err)
	}

	return actions.NewReturnValues(true,
		actions.NewStringReturnField("lock_name", name),
		actions.NewStringReturnField("expires", expires),
	), nil, nil
}

func (l *lockActions) unlockUser(ctx context.Context, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
	user, err := l.requireUser(ctx, args)
	if err != nil {
		return nil, nil, err
	}

	target := types.LockTarget{User: user}
	filter := &types.LockFilter{Targets: []*types.LockTarget{&target}}
	deleted := 0
	startKey := ""
	for {
		locks, next, err := l.api.ListLocks(ctx, 0, startKey, filter)
		if err != nil {
			return nil, nil, fmt.Errorf("baton-teleport: failed to list locks for user %q: %w", user, err)
		}
		for _, lock := range locks {
			// Only exact user locks; compound targets (e.g. user+role) are not ours to remove.
			if !lock.Target().Equals(target) {
				continue
			}
			if err := l.api.DeleteLock(ctx, lock.GetName()); err != nil && !trace.IsNotFound(err) {
				return nil, nil, fmt.Errorf("baton-teleport: failed to delete lock %q for user %q: %w", lock.GetName(), user, err)
			}
			deleted++
		}
		if next == "" {
			break
		}
		startKey = next
	}

	return actions.NewReturnValues(true,
		actions.NewStringReturnField("deleted_locks", fmt.Sprintf("%d", deleted)),
	), nil, nil
}
