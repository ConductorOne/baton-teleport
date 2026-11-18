package connector

import (
	"context"
	"errors"
	"regexp"
	"strconv"
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
	pageSize  int
	listErr   error
	deleteErr map[string]error
	upserted  []types.Lock
	deleted   []string
	getCalls  int

	currentUser    string
	currentUserErr error
	identityCN     string
}

func (f *fakeLockAPI) GetCurrentUser(_ context.Context) (types.User, error) {
	if f.currentUserErr != nil {
		return nil, f.currentUserErr
	}
	name := f.currentUser
	if name == "" {
		name = "connector-svc"
	}
	return types.NewUser(name)
}

func (f *fakeLockAPI) IdentityCommonName() string {
	return f.identityCN
}

func (f *fakeLockAPI) GetUser(_ context.Context, name string, _ bool) (types.User, error) {
	f.getCalls++
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
	if err := f.deleteErr[name]; err != nil {
		return err
	}
	f.deleted = append(f.deleted, name)
	return nil
}

// ListLocks pages over f.locks; the start key is the index of the next lock.
func (f *fakeLockAPI) ListLocks(_ context.Context, _ int, startKey string, _ *types.LockFilter) ([]types.Lock, string, error) {
	if f.listErr != nil {
		return nil, "", f.listErr
	}
	start := 0
	if startKey != "" {
		n, err := strconv.Atoi(startKey)
		if err != nil {
			return nil, "", err
		}
		start = n
	}
	if f.pageSize <= 0 || start+f.pageSize >= len(f.locks) {
		return f.locks[start:], "", nil
	}
	end := start + f.pageSize
	return f.locks[start:end], strconv.Itoa(end), nil
}

func mustLock(t *testing.T, name string, target types.LockTarget) types.Lock {
	l, err := types.NewLock(name, types.LockSpecV2{Target: target})
	require.NoError(t, err)
	return l
}

func mustLockExpiring(t *testing.T, name string, target types.LockTarget, expires *time.Time) types.Lock {
	l, err := types.NewLock(name, types.LockSpecV2{Target: target, Expires: expires, Message: "existing"})
	require.NoError(t, err)
	return l
}

var lockTestNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

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
		{
			name:        "duration string",
			args:        map[string]any{"user": "alice", "duration": "2h", "message": "bye"},
			api:         &fakeLockAPI{users: map[string]bool{"alice": true}},
			wantExpires: ptr(fixed.Add(2 * time.Hour)),
			wantMessage: "bye",
		},
		{
			name:        "duration seconds number",
			args:        map[string]any{"user": "alice", "duration": 90.0},
			api:         &fakeLockAPI{users: map[string]bool{"alice": true}},
			wantExpires: ptr(fixed.Add(90 * time.Second)),
			wantMessage: defaultLockMessage,
		},
		{name: "missing user", args: map[string]any{}, api: &fakeLockAPI{}, wantErr: "baton-teleport:"},
		{name: "blank user", args: map[string]any{"user": "  "}, api: &fakeLockAPI{}, wantErr: "user is required"},
		{name: "unknown user", args: map[string]any{"user": "bob"}, api: &fakeLockAPI{users: map[string]bool{}}, wantErr: "not found"},
		{name: "get user error", args: map[string]any{"user": "bob"}, api: &fakeLockAPI{getErr: errors.New("boom")}, wantErr: "failed to get user"},
		{name: "negative duration", args: map[string]any{"user": "alice", "duration": "-1h"}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantErr: "must be positive"},
		{name: "zero duration", args: map[string]any{"user": "alice", "duration": "0s"}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantErr: "must be positive"},
		{name: "zero number duration", args: map[string]any{"user": "alice", "duration": 0.0}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantErr: "positive number of seconds"},
		{name: "unparseable duration", args: map[string]any{"user": "alice", "duration": "soon"}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantErr: "invalid duration"},
		{name: "overflowing seconds", args: map[string]any{"user": "alice", "duration": 1e10}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantErr: "no greater than"},
		{name: "sub-nanosecond seconds", args: map[string]any{"user": "alice", "duration": 1e-10}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantErr: "too small"},
		{
			name:        "max-ish seconds ok",
			args:        map[string]any{"user": "alice", "duration": 1e9},
			api:         &fakeLockAPI{users: map[string]bool{"alice": true}},
			wantExpires: ptr(fixed.Add(1e9 * time.Second)),
			wantMessage: defaultLockMessage,
		},
		{name: "bool duration", args: map[string]any{"user": "alice", "duration": true}, api: &fakeLockAPI{users: map[string]bool{"alice": true}}, wantErr: "string or a number"},
		{
			name:    "access denied on upsert",
			args:    map[string]any{"user": "alice"},
			api:     &fakeLockAPI{users: map[string]bool{"alice": true}, upsertErr: trace.AccessDenied("nope")},
			wantErr: "missing lock RBAC",
		},
		{name: "access denied on get user", args: map[string]any{"user": "alice"}, api: &fakeLockAPI{getErr: trace.AccessDenied("nope")}, wantErr: "missing lock RBAC"},
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
			require.Regexp(t, lockNameRegexp, l.GetName())
			require.Equal(t, l.GetName(), out.GetFields()["lock_name"].GetStringValue())
		})
	}
}

var lockNameRegexp = regexp.MustCompile(`^c1-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestLockUserCreatesDistinctLocks(t *testing.T) {
	api := &fakeLockAPI{users: map[string]bool{"alice": true}}
	la := &lockActions{api: api, now: func() time.Time { return lockTestNow }}
	first, _, err := la.lockUser(context.Background(), newArgs(t, map[string]any{"user": "alice", "duration": "1h"}))
	require.NoError(t, err)
	second, _, err := la.lockUser(context.Background(), newArgs(t, map[string]any{"user": "alice", "duration": "2h"}))
	require.NoError(t, err)
	require.Len(t, api.upserted, 2)
	require.NotEqual(t, api.upserted[0].GetName(), api.upserted[1].GetName())
	require.Regexp(t, lockNameRegexp, api.upserted[0].GetName())
	require.Regexp(t, lockNameRegexp, api.upserted[1].GetName())
	require.Equal(t, api.upserted[0].GetName(), first.GetFields()["lock_name"].GetStringValue())
	require.Equal(t, api.upserted[1].GetName(), second.GetFields()["lock_name"].GetStringValue())
	require.Equal(t, lockTestNow.Add(time.Hour).Format(time.RFC3339), first.GetFields()["expires"].GetStringValue())
	require.Equal(t, lockTestNow.Add(2*time.Hour).Format(time.RFC3339), second.GetFields()["expires"].GetStringValue())
}

func TestLockUserSelfLockGuard(t *testing.T) {
	tests := []struct {
		name        string
		user        string
		api         *fakeLockAPI
		wantErr     string
		wantLocked  bool
		wantGetUser int
	}{
		{name: "own user refused", user: "svc", api: &fakeLockAPI{currentUser: "svc", users: map[string]bool{"svc": true}}, wantErr: "refusing to lock svc"},
		{name: "different user allowed", user: "alice", api: &fakeLockAPI{currentUser: "svc", users: map[string]bool{"alice": true}}, wantLocked: true, wantGetUser: 1},
		{name: "case differing name allowed", user: "SVC", api: &fakeLockAPI{currentUser: "svc", users: map[string]bool{"SVC": true}}, wantLocked: true, wantGetUser: 1},
		{
			name:    "current user error fails closed",
			user:    "alice",
			api:     &fakeLockAPI{currentUserErr: errors.New("boom"), users: map[string]bool{"alice": true}},
			wantErr: "cannot determine the connector's own Teleport user",
		},
		{
			name:    "current user access denied fails closed with hint",
			user:    "alice",
			api:     &fakeLockAPI{currentUserErr: trace.AccessDenied("nope"), users: map[string]bool{"alice": true}},
			wantErr: "missing lock RBAC",
		},
		{
			name:    "certificate common name match refused",
			user:    "cert-svc",
			api:     &fakeLockAPI{currentUser: "svc", identityCN: "cert-svc", users: map[string]bool{"cert-svc": true}},
			wantErr: "connector's own Teleport user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			la := &lockActions{api: tt.api, now: func() time.Time { return lockTestNow }}
			_, _, err := la.lockUser(context.Background(), newArgs(t, map[string]any{"user": tt.user}))
			require.Equal(t, tt.wantGetUser, tt.api.getCalls)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Empty(t, tt.api.upserted)
				return
			}
			require.NoError(t, err)
			require.Len(t, tt.api.upserted, 1)
		})
	}
}

func TestOwnUserResolvedOnce(t *testing.T) {
	api := &countingCurrentUserAPI{fakeLockAPI: fakeLockAPI{users: map[string]bool{"alice": true}}}
	la := &lockActions{api: api, now: func() time.Time { return lockTestNow }}
	for i := 0; i < 3; i++ {
		_, _, err := la.lockUser(context.Background(), newArgs(t, map[string]any{"user": "alice"}))
		require.NoError(t, err)
	}
	require.Equal(t, 1, api.currentCalls)
}

type countingCurrentUserAPI struct {
	fakeLockAPI
	currentCalls int
}

func (c *countingCurrentUserAPI) GetCurrentUser(ctx context.Context) (types.User, error) {
	c.currentCalls++
	return c.fakeLockAPI.GetCurrentUser(ctx)
}

func ptr[T any](v T) *T { return &v }

func TestInvalidDurationDoesNotCallGetUser(t *testing.T) {
	api := &fakeLockAPI{users: map[string]bool{"alice": true}}
	la := &lockActions{api: api, now: time.Now}
	_, _, err := la.lockUser(context.Background(), newArgs(t, map[string]any{"user": "alice", "duration": "-1h"}))
	require.Error(t, err)
	require.Zero(t, api.getCalls)
}

func TestUnlockUser(t *testing.T) {
	userLock := func(n string) types.Lock { return mustLock(t, n, types.LockTarget{User: "alice"}) }
	tests := []struct {
		name        string
		api         *fakeLockAPI
		user        any
		wantErr     string
		wantDeleted []string
		wantCount   float64
	}{
		{
			name: "deletes only exact user locks",
			api: &fakeLockAPI{users: map[string]bool{"alice": true}, locks: []types.Lock{
				userLock("l1"), mustLock(t, "l2", types.LockTarget{User: "alice", Role: "admin"}), userLock("l3"),
			}},
			user: "alice", wantDeleted: []string{"l1", "l3"}, wantCount: 2,
		},
		{
			name: "deletes exact locks of different names",
			api: &fakeLockAPI{locks: []types.Lock{
				userLock("c1-alice"), userLock("11111111-2222-3333-4444-555555555555"), userLock("manual lock"),
			}},
			user: "alice", wantDeleted: []string{"c1-alice", "11111111-2222-3333-4444-555555555555", "manual lock"}, wantCount: 3,
		},
		{
			name: "skips expired locks and keeps unexpired and permanent ones",
			api: &fakeLockAPI{locks: []types.Lock{
				mustLockExpiring(t, "expired", types.LockTarget{User: "alice"}, ptr(lockTestNow.Add(-time.Minute))),
				mustLockExpiring(t, "at-now", types.LockTarget{User: "alice"}, ptr(lockTestNow)),
				mustLockExpiring(t, "future", types.LockTarget{User: "alice"}, ptr(lockTestNow.Add(time.Hour))),
				userLock("permanent"),
			}},
			user: "alice", wantDeleted: []string{"future", "permanent"}, wantCount: 2,
		},
		{
			name: "user absent but lock present",
			api:  &fakeLockAPI{users: map[string]bool{}, locks: []types.Lock{userLock("l1")}},
			user: "alice", wantDeleted: []string{"l1"}, wantCount: 1,
		},
		{
			name: "multi page",
			api:  &fakeLockAPI{pageSize: 2, locks: []types.Lock{userLock("l1"), userLock("l2"), userLock("l3"), userLock("l4"), userLock("l5")}},
			user: "alice", wantDeleted: []string{"l1", "l2", "l3", "l4", "l5"}, wantCount: 5,
		},
		{
			name: "no matching locks",
			api:  &fakeLockAPI{locks: []types.Lock{mustLock(t, "x", types.LockTarget{User: "bob"})}},
			user: "alice", wantCount: 0,
		},
		{
			name: "delete not found tolerated and not counted",
			api: &fakeLockAPI{locks: []types.Lock{userLock("l1"), userLock("l2")},
				deleteErr: map[string]error{"l1": trace.NotFound("gone")}},
			user: "alice", wantDeleted: []string{"l2"}, wantCount: 1,
		},
		{
			name: "delete other error",
			api: &fakeLockAPI{locks: []types.Lock{userLock("l1")},
				deleteErr: map[string]error{"l1": errors.New("boom")}},
			user: "alice", wantErr: "failed to delete lock",
		},
		{
			name: "delete access denied",
			api: &fakeLockAPI{locks: []types.Lock{userLock("l1")},
				deleteErr: map[string]error{"l1": trace.AccessDenied("no")}},
			user: "alice", wantErr: "missing lock RBAC",
		},
		{name: "list error", api: &fakeLockAPI{listErr: errors.New("boom")}, user: "alice", wantErr: "failed to list locks"},
		{name: "list access denied", api: &fakeLockAPI{listErr: trace.AccessDenied("no")}, user: "alice", wantErr: "missing lock RBAC"},
		{name: "missing user arg", api: &fakeLockAPI{}, user: nil, wantErr: "baton-teleport:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := map[string]any{}
			if tt.user != nil {
				m["user"] = tt.user
			}
			la := &lockActions{api: tt.api, now: func() time.Time { return lockTestNow }}
			out, _, err := la.unlockUser(context.Background(), newArgs(t, m))
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Zero(t, tt.api.getCalls, "unlock must not require the user to exist")
			require.Equal(t, tt.wantDeleted, tt.api.deleted)
			require.EqualValues(t, tt.wantCount, out.GetFields()["deleted_locks"].GetNumberValue())
			var names []string
			for _, v := range out.GetFields()["deleted_lock_names"].GetListValue().GetValues() {
				names = append(names, v.GetStringValue())
			}
			require.Equal(t, tt.wantDeleted, names)
		})
	}
}

func TestNilClient(t *testing.T) {
	la := &lockActions{}
	args := newArgs(t, map[string]any{"user": "alice"})
	_, _, err := la.lockUser(context.Background(), args)
	require.ErrorContains(t, err, "not configured")
	_, _, err = la.unlockUser(context.Background(), args)
	require.ErrorContains(t, err, "not configured")
}

// The C1 "connector action" automation step rejects schemas that carry the
// account enable/disable lifecycle action types, so these actions must not set
// any lifecycle ActionType.
func TestSchemasHaveNoLifecycleActionType(t *testing.T) {
	for _, s := range []*v2.BatonActionSchema{lockUserSchema(), unlockUserSchema()} {
		require.Empty(t, s.GetActionType(), s.GetName())
	}
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
