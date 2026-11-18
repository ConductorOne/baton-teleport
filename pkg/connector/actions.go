package connector

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/conductorone/baton-sdk/pkg/actions"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/google/uuid"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
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

	lockNamePrefix = "c1-"

	defaultLockMessage = "This user has been locked by ConductorOne."
)

// lockAPI is the subset of the Teleport client used by the lock actions.
type lockAPI interface {
	GetUser(ctx context.Context, name string, withSecrets bool) (types.User, error)
	GetCurrentUser(ctx context.Context) (types.User, error)
	IdentityCommonName() string
	UpsertLock(ctx context.Context, lock types.Lock) error
	DeleteLock(ctx context.Context, name string) error
	ListLocks(ctx context.Context, limit int, startKey string, filter *types.LockFilter) ([]types.Lock, string, error)
}

var _ connectorbuilder.GlobalActionProvider = (*Connector)(nil)
var _ lockAPI = (*client.TeleportClient)(nil)

type lockActions struct {
	api lockAPI
	now func() time.Time

	ownMu    sync.Mutex
	ownUsers map[string]struct{}
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
			"Each call creates a new lock named c1-<uuid>, so repeat calls create separate locks with their own expiry. " +
			"The user stays blocked until the last non-expired lock expires or is removed with unlock_user. " +
			"The action refuses to lock the connector's own Teleport user.",
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
		Description: "Removes all non-expired Teleport locks that target exactly this user, whatever their name or creator. " +
			"Expired locks and locks on roles, devices or compound targets are left untouched.",
		Arguments: []*config.Field{userArgField()},
		ReturnTypes: []*config.Field{
			config.Field_builder{
				Name:        "deleted_locks",
				DisplayName: "Deleted locks",
				Description: "Number of locks removed.",
				IntField:    &config.IntField{},
			}.Build(),
			config.Field_builder{
				Name:             "deleted_lock_names",
				DisplayName:      "Deleted lock names",
				Description:      "Names of the locks removed.",
				StringSliceField: &config.StringSliceField{},
			}.Build(),
		},
	}
}

// parseLockDuration parses a non-empty Go duration string.
func parseLockDuration(raw string) (time.Duration, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("baton-teleport: invalid %s %q (use a Go duration such as 30m or 24h): %w", argDuration, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("baton-teleport: %s must be positive, got %q", argDuration, raw)
	}
	return d, nil
}

// maxDurationSeconds keeps the seconds-to-Duration conversion within int64 nanoseconds.
const maxDurationSeconds = float64(math.MaxInt64/int64(time.Second)) - 1

// durationArg reads the optional duration. Accepts a string, or a number of seconds.
// It never returns a non-positive duration for a supplied value, so a bad value cannot
// silently become a permanent lock.
func durationArg(args *structpb.Struct) (time.Duration, bool, error) {
	v, ok := args.GetFields()[argDuration]
	if !ok {
		return 0, false, nil
	}
	switch k := v.GetKind().(type) {
	case *structpb.Value_NullValue:
		return 0, false, nil
	case *structpb.Value_NumberValue:
		secs := k.NumberValue
		if math.IsNaN(secs) || secs <= 0 || secs > maxDurationSeconds {
			return 0, false, fmt.Errorf("baton-teleport: %s must be a positive number of seconds no greater than %.0f, got %v", argDuration, maxDurationSeconds, secs)
		}
		d := time.Duration(secs * float64(time.Second))
		if d <= 0 {
			return 0, false, fmt.Errorf("baton-teleport: %s of %v seconds is too small (minimum is 1ns)", argDuration, secs)
		}
		return d, true, nil
	case *structpb.Value_StringValue:
		raw := strings.TrimSpace(k.StringValue)
		if raw == "" {
			return 0, false, nil
		}
		d, err := parseLockDuration(raw)
		return d, err == nil, err
	default:
		return 0, false, fmt.Errorf("baton-teleport: %s must be a string or a number of seconds", argDuration)
	}
}

// requireUserArg validates the user argument and that a client is configured.
func (l *lockActions) requireUserArg(args *structpb.Struct) (string, error) {
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
	return user, nil
}

// wrapAPIError adds an actionable hint for RBAC failures.
func wrapAPIError(err error, format string, a ...any) error {
	msg := fmt.Sprintf(format, a...)
	if trace.IsAccessDenied(err) {
		return fmt.Errorf(
			"baton-teleport: %s: missing lock RBAC; the connector role needs rules for resource 'lock' "+
				"with verbs [list, create, read, update, delete] (and read on 'user'): %w",
			msg, err,
		)
	}
	return fmt.Errorf("baton-teleport: %s: %w", msg, err)
}

func (l *lockActions) lockUser(ctx context.Context, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
	user, err := l.requireUserArg(args)
	if err != nil {
		return nil, nil, err
	}
	d, hasDuration, err := durationArg(args)
	if err != nil {
		return nil, nil, err
	}
	if err := l.refuseOwnUser(ctx, user); err != nil {
		return nil, nil, err
	}
	if _, err := l.api.GetUser(ctx, user, false); err != nil {
		if trace.IsNotFound(err) {
			return nil, nil, fmt.Errorf("baton-teleport: user %q not found: %w", user, err)
		}
		return nil, nil, wrapAPIError(err, "failed to get user %q", user)
	}
	message, _ := actions.GetStringArg(args, argMessage)
	if strings.TrimSpace(message) == "" {
		message = defaultLockMessage
	}

	spec := types.LockSpecV2{
		Target:  types.LockTarget{User: user},
		Message: message,
	}
	var expires *time.Time
	if hasDuration {
		t := l.now().UTC().Add(d)
		spec.Expires = &t
		expires = &t
	}

	name := lockNamePrefix + uuid.NewString()
	lock, err := types.NewLock(name, spec)
	if err != nil {
		return nil, nil, fmt.Errorf("baton-teleport: failed to build lock for user %q: %w", user, err)
	}
	if err := l.api.UpsertLock(ctx, lock); err != nil {
		return nil, nil, wrapAPIError(err, "failed to lock user %q", user)
	}

	return lockUserResult(name, expires), nil, nil
}

func (l *lockActions) ownUsernames(ctx context.Context) (map[string]struct{}, error) {
	l.ownMu.Lock()
	defer l.ownMu.Unlock()
	if l.ownUsers != nil {
		return l.ownUsers, nil
	}
	current, err := l.api.GetCurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	names := map[string]struct{}{current.GetName(): {}}
	if cn := l.api.IdentityCommonName(); cn != "" {
		names[cn] = struct{}{}
	}
	l.ownUsers = names
	return names, nil
}

func (l *lockActions) refuseOwnUser(ctx context.Context, user string) error {
	own, err := l.ownUsernames(ctx)
	if err != nil {
		return wrapAPIError(err, "refusing to lock %q: cannot determine the connector's own Teleport user", user)
	}
	if _, ok := own[user]; ok {
		return fmt.Errorf("baton-teleport: refusing to lock %s: it is the connector's own Teleport user; locking it would block the connector credential", user)
	}
	return nil
}

func lockUserResult(name string, expiry *time.Time) *structpb.Struct {
	var expires string
	if expiry != nil {
		expires = expiry.UTC().Format(time.RFC3339)
	}
	return actions.NewReturnValues(true,
		actions.NewStringReturnField("lock_name", name),
		actions.NewStringReturnField("expires", expires),
	)
}

func (l *lockActions) unlockUser(ctx context.Context, args *structpb.Struct) (*structpb.Struct, annotations.Annotations, error) {
	// The user need not exist: this must still be able to clear a stale lock.
	user, err := l.requireUserArg(args)
	if err != nil {
		return nil, nil, err
	}

	target := types.LockTarget{User: user}
	filter := &types.LockFilter{Targets: []*types.LockTarget{&target}}
	logger := ctxzap.Extract(ctx)
	now := l.now()
	deletedNames := []string{}
	startKey := ""
	for {
		locks, next, err := l.api.ListLocks(ctx, 0, startKey, filter)
		if err != nil {
			return nil, nil, wrapAPIError(err, "failed to list locks for user %q", user)
		}
		for _, lock := range locks {
			// Only exact user locks; compound targets (e.g. user+role) are not ours to remove.
			if !lock.Target().Equals(target) {
				continue
			}
			if expiry := lock.LockExpiry(); expiry != nil && !expiry.After(now) {
				logger.Debug("skipping expired lock", zap.String("user", user), zap.String("lock_name", lock.GetName()))
				continue
			}
			err := l.api.DeleteLock(ctx, lock.GetName())
			switch {
			case err == nil:
				deletedNames = append(deletedNames, lock.GetName())
				logger.Info("deleted lock", zap.String("user", user), zap.String("lock_name", lock.GetName()))
			case trace.IsNotFound(err):
				// Already gone (expired or removed concurrently).
			default:
				return nil, nil, wrapAPIError(err, "failed to delete lock %q for user %q", lock.GetName(), user)
			}
		}
		if next == "" {
			break
		}
		startKey = next
	}

	return actions.NewReturnValues(true,
		actions.NewNumberReturnField("deleted_locks", float64(len(deletedNames))),
		actions.NewStringListReturnField("deleted_lock_names", deletedNames),
	), nil, nil
}
