package connector

import (
	"context"
	"errors"
	"testing"
	"time"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/actions"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

type fakeLockAPI struct {
	users     map[string]bool
	getErr    error
	upsertErr error
	locks     []types.Lock
	upserted  []types.Lock
	deleted   []string
}

func (f *fakeLockAPI) GetUser(_ context.Context, name string, _ bool) (types.User, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if !f.users[name] {
		return nil, trace.NotFound("user %q not found", name)
	}
	return types.NewUser(name)
}

func (f *fakeLockAPI) UpsertLock(_ context.Context, l types.Lock) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.upserted = append(f.upserted, l)
	return nil
}

func (f *fakeLockAPI) DeleteLock(_ context.Context, name string) error {
	f.deleted = append(f.deleted, name)
	return nil
}

func (f *fakeLockAPI) ListLocks(_ context.Context, _ int, _ string, _ *types.LockFilter) ([]types.Lock, string, error) {
	return f.locks, "", nil
}

func mustLock(t *testing.T, name string, target types.LockTarget) types.Lock {
	l, err := types.NewLock(name, types.LockSpecV2{Target: target})
	require.NoError(t, err)
	return l
}

func newArgs(t *testing.T, m map[string]any) *structpb.Struct {
	s, err := structpb.NewStruct(m)
	require.NoError(t, err)
	return s
}

func TestLockUser(t *testing.T) {
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name        string
		args        map[string]any
		api         *fakeLockAPI
		wantErr     string
		wantExpires *time.Time
		wantMessage string
	}{
		{name: "no expiry default message", args: map[string]any{"user": "alice"}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantMessage: defaultLockMessage},
		{name: "duration string", args: map[string]any{"user": "alice", "duration": "2h", "message": "bye"}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantExpires: ptr(fixed.Add(2 * time.Hour)), wantMessage: "bye"},
		{name: "duration seconds number", args: map[string]any{"user": "alice", "duration": 90.0}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantExpires: ptr(fixed.Add(90 * time.Second)), wantMessage: defaultLockMessage},
		{name: "missing user", args: map[string]any{}, api: &fakeLockAPI{}, wantErr: "baton-teleport:"},
		{name: "blank user", args: map[string]any{"user": "  "}, api: &fakeLockAPI{}, wantErr: "user is required"},
		{name: "unknown user", args: map[string]any{"user": "bob"}, api: &fakeLockAPI{users: map[string]bool{}}, wantErr: "not found"},
		{name: "get user error", args: map[string]any{"user": "bob"}, api: &fakeLockAPI{getErr: errors.New("boom")}, wantErr: "failed to get user"},
		{name: "negative duration", args: map[string]any{"user": "alice", "duration": "-1h"}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantErr: "must be positive"},
		{name: "zero duration", args: map[string]any{"user": "alice", "duration": "0s"}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantErr: "must be positive"},
		{name: "zero number duration", args: map[string]any{"user": "alice", "duration": 0.0}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantErr: "positive number of seconds"},
		{name: "unparseable duration", args: map[string]any{"user": "alice", "duration": "soon"}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantErr: "invalid duration"},
		{name: "upsert error", args: map[string]any{"user": "alice"}, api: &fakeLockAPI{users: map[string]bool{"alice": true}, upsertErr: errors.New("denied")}, wantErr: "failed to lock user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			la := &lockActions{api: tt.api, now: func() time.Time { return fixed }}
			out, _, err := la.lockUser(context.Background(), newArgs(t, tt.args))
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Empty(t, tt.api.upserted)
				return
			}
			require.NoError(t, err)
			require.Len(t, tt.api.upserted, 1)
			l := tt.api.upserted[0]
			require.Equal(t, "alice", l.Target().User)
			require.Equal(t, tt.wantMessage, l.Message())
			if tt.wantExpires == nil {
				require.Nil(t, l.LockExpiry())
			} else {
				require.NotNil(t, l.LockExpiry())
				require.True(t, tt.wantExpires.Equal(*l.LockExpiry()))
			}
			require.True(t, out.GetFields()["success"].GetBoolValue())
			require.Equal(t, l.GetName(), out.GetFields()["lock_name"].GetStringValue())
		})
	}
}

func ptr[T any](v T) *T { return &v }

func TestUnlockUser(t *testing.T) {
	api := &fakeLockAPI{
		users: map[string]bool{"alice": true},
		locks: []types.Lock{
			mustLock(t, "l1", types.LockTarget{User: "alice"}),
			mustLock(t, "l2", types.LockTarget{User: "alice", Role: "admin"}),
			mustLock(t, "l3", types.LockTarget{User: "alice"}),
		},
	}
	la := &lockActions{api: api, now: time.Now}
	out, _, err := la.unlockUser(context.Background(), newArgs(t, map[string]any{"user": "alice"}))
	require.NoError(t, err)
	require.Equal(t, []string{"l1", "l3"}, api.deleted)
	require.Equal(t, "2", out.GetFields()["deleted_locks"].GetStringValue())

	_, _, err = la.unlockUser(context.Background(), newArgs(t, map[string]any{"user": "nobody"}))
	require.ErrorContains(t, err, "not found")

	_, _, err = (&lockActions{}).unlockUser(context.Background(), newArgs(t, map[string]any{"user": "alice"}))
	require.ErrorContains(t, err, "not configured")
}

type recordingRegistry struct{ names []string }

func (r *recordingRegistry) Register(_ context.Context, s *v2.BatonActionSchema, _ actions.ActionHandler) error {
	r.names = append(r.names, s.GetName())
	return nil
}

func (r *recordingRegistry) RegisterAction(ctx context.Context, _ string, s *v2.BatonActionSchema, h actions.ActionHandler) error {
	return r.Register(ctx, s, h)
}

func TestGlobalActionsRegistered(t *testing.T) {
	reg := &recordingRegistry{}
	require.NoError(t, (&Connector{}).GlobalActions(context.Background(), reg))
	require.ElementsMatch(t, []string{"lock_user", "unlock_user"}, reg.names)
}
