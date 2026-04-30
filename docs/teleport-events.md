# Teleport Event Feed Reference

This document describes all Teleport audit events handled by the baton-teleport connector's event feeds, when they fire, and what ConductorOne events they produce.

Verified against **Teleport v18** live API (2026-02-27).

---

## Event Feeds Overview

The connector registers **two** event feeds:

| Feed ID | Event Types Emitted | Purpose |
|---------|-------------------|---------|
| `teleport_usage_events` | `UsageEvent` | Tracks user login activity so C1 can derive last-login timestamps |
| `teleport_audit_events` | `ResourceChangeEvent`, `CreateGrantEvent`, `CreateRevokeEvent` | Tracks resource lifecycle changes for incremental sync |

Both feeds use `SearchEvents` with `EventOrderAscending` and share a common pagination cursor (`eventPageCursor`) that tracks `StartAt`, `LatestEventSeen`, and Teleport's opaque `LastKey`.

---

## Teleport Naming Convention

> **Teleport is inconsistent between resource types.** Always verify event type strings with the live API before adding new types.

| Resource | Create | Update | Delete |
|----------|--------|--------|--------|
| User | `user.create` | `user.update` | `user.delete` |
| Role | `role.created` | `role.updated` | `role.deleted` |
| App | `app.create` | `app.update` | `app.delete` |
| Database | `db.create` | `db.update` | `db.delete` |
| Access Request | `access_request.create` | `access_request.review` / `access_request.update` | `access_request.delete` |
| Lock | `lock.created` | — | `lock.deleted` |

**Key rules:**
- User/app/db events use **present tense** (`user.create`, `app.delete`).
- Role/lock events use **past tense** (`role.created`, `lock.deleted`).
- User events use **separate** event types: `CreateUser()` → `user.create`, `UpdateUser()` → `user.update`. Both carry the complete `Roles` list.
- Role events also use **separate** types: `CreateRole()` → `role.created`, role modification → `role.updated` (code T9002I).
- There is **no** `user.updated` event (past tense). The present-tense `user.update` is the correct string.

---

## Usage Event Feed (`teleport_usage_events`)

### `user.login`

| Field | Value |
|-------|-------|
| Teleport event string | `user.login` |
| Go type | `*events.UserLogin` |
| When fired | Every time a user authenticates to the Teleport cluster |
| Filter | Only **successful** logins are emitted (`Status.Success == true`) |
| C1 event type | `UsageEvent` |
| Target resource | Teleport cluster (`teleport_cluster` / cluster name) |
| Actor resource | The user who logged in (`user` / username) |

**Purpose:** ConductorOne derives last-login timestamps from usage events. This avoids querying audit logs during `List()`.

---

## Audit Event Feed (`teleport_audit_events`)

### User Events

#### `user.create`

| Field | Value |
|-------|-------|
| Teleport event string | `user.create` |
| Go type | `*events.UserCreate` |
| When fired | User is **created** via `CreateUser()` |
| C1 events emitted | 1 `ResourceChangeEvent` (user) + 1 `CreateGrantEvent` per role in `Roles` field |
| Guard | Skipped if `ResourceMetadata.Name` is empty |

**Behavior:**
- The `Roles` field carries the **complete** post-operation role list.
- `CreateGrantEvent`s represent the initial grants assigned at creation time.
- Empty role names in the `Roles` slice are skipped.

#### `user.update`

| Field | Value |
|-------|-------|
| Teleport event string | `user.update` |
| Go type | `*events.UserUpdate` |
| When fired | User is **modified** via `UpdateUser()` |
| C1 events emitted | 1 `ResourceChangeEvent` (user) + 1 `CreateGrantEvent` per role in `Roles` field |
| Guard | Skipped if `ResourceMetadata.Name` is empty |

**Behavior:**
- Same payload shape as `UserCreate` — carries the complete new role list.
- `CreateGrantEvent`s are idempotent — C1 deduplicates already-existing grants, and any newly added role is picked up immediately.
- Empty role names in the `Roles` slice are skipped.

**Limitation — role-membership revocations are not emitted incrementally:**

When a role is **removed** from a user (e.g. roles go from `[A, B, C]` to `[A, B]`), the connector does **not** emit a `CreateRevokeEvent` for the removed role. The removed role-membership grant remains visible in ConductorOne until the next full sync reconciles it.

Why:
- Teleport's `user.update` audit payload only carries the **post-update** role list (`Roles` field). It does not include the previous role list for the target user.
- The event's `UserMetadata.user_roles` field looks tempting but is unreliable: it represents the **actor's** roles (the user who performed the update), not the target user's previous roles. In the common case where an admin edits a different user, `user_roles` is the admin's roles and using it for a diff would emit nonsense revokes.
- Without a reliable "previous roles" signal, the connector cannot identify which role-membership was removed, so it skips revoke emission entirely and lets the next full-sync cycle reconcile the state.

What this means in practice:
- **Role additions** propagate immediately via `CreateGrantEvent`.
- **Role removals** are visible only after the next full sync (typically minutes to hours, depending on the sync schedule).
- Profile changes (status, email, locks) propagate immediately via `ResourceChangeEvent`.

> **Live API verification (2026-02-27):** `CreateUser()` fires `user.create` (`*events.UserCreate`), `UpdateUser()` fires `user.update` (`*events.UserUpdate`). These are **separate** event types, not upsert semantics.

---

### Role Events

#### `role.created`

| Field | Value |
|-------|-------|
| Teleport event string | `role.created` |
| Go type | `*events.RoleCreate` |
| When fired | Role is **created** |
| C1 events emitted | 1 `ResourceChangeEvent` (role) |
| Guard | Skipped if `ResourceMetadata.Name` is empty |

#### `role.updated`

| Field | Value |
|-------|-------|
| Teleport event string | `role.updated` |
| Go type | `*events.RoleUpdate` |
| When fired | Role is **modified** (code T9002I) |
| C1 events emitted | 1 `ResourceChangeEvent` (role) |
| Guard | Skipped if `ResourceMetadata.Name` is empty |

> **Live API verification (2026-02-27):** Role modifications fire a separate `role.updated` event (code T9002I, Go type `*events.RoleUpdate`), distinct from the `role.created` event.

---

### Access Request Events

Teleport access requests use multiple event types for the request lifecycle. All state-change events (`review`, `update`, `expire`) use the **same Go type** (`*events.AccessRequestCreate`) but with different payload contents.

#### `access_request.review` (Reviewer Action)

| Field | Value |
|-------|-------|
| Teleport event string | `access_request.review` |
| Go type | `*events.AccessRequestCreate` |
| When fired | Reviewer **approves** or **denies** an access request |
| Payload | `UserMetadata.User` ❌ **empty**, `Roles` ❌ **empty**, `RequestState` ✅, `RequestID` ✅ |
| C1 events emitted | If `APPROVED`: 1 `CreateGrantEvent` per role. If `DENIED`: nothing. |
| Resolution | Looks up original request by `RequestID` via `GetAccessRequests` API |

#### `access_request.update` (State Transition)

| Field | Value |
|-------|-------|
| Teleport event string | `access_request.update` |
| Go type | `*events.AccessRequestCreate` |
| When fired | Access request state transitions (e.g. `APPROVED`) |
| Payload | `UserMetadata.User` ❌ **empty**, `RequestState` ✅, `RequestID` ✅ |
| C1 events emitted | If `APPROVED`: 1 `CreateGrantEvent` per role. If `DENIED`: nothing. |
| Resolution | Same lookup as `access_request.review` |

#### `access_request.expire` (Expiration)

| Field | Value |
|-------|-------|
| Teleport event string | `access_request.expire` |
| Go type | `*events.AccessRequestCreate` |
| When fired | A time-limited access request **expires** and temporary roles are revoked |
| Payload | Minimal — `RequestID` ✅, `User`/`Roles` ❌ **empty** |
| C1 events emitted | 1 `CreateRevokeEvent` per role (if state is `EXPIRED`) |
| Resolution | Same lookup as `access_request.review` |

**Behavior (shared across review / update / expire):**
- All three event types use the **same Go type** (`*events.AccessRequestCreate`) with `User` and `Roles` fields **empty**. Only `RequestID` and `RequestState` are populated.
- The connector resolves the original request by `RequestID` via `GetAccessRequests(ctx, AccessRequestFilter{ID: requestID})` to recover the user and roles.
- **APPROVED**: emits `CreateGrantEvent` per role so C1 picks up temporary grants immediately.
- **EXPIRED**: emits `CreateRevokeEvent` per role so C1 knows the temporary grants are gone.
- **DENIED** and other states: nothing emitted (no access change occurred).
- No `ResourceChangeEvent` is emitted because the user's `Get()` only returns profile data (name, email, status) which hasn't changed — role grants are communicated via grant/revoke events.
- If the original request cannot be resolved (deleted, error), the event is silently skipped with a debug log.

**Why these handlers are NOT redundant with `user.create` / `user.update`:**
- Teleport access requests grant **temporary** access via certificate reissuance — they do NOT modify the user record.
- Approving an access request does **not** fire a `user.create` or `user.update` audit event because no `CreateUser`/`UpdateUser` call happens.
- These handlers are the **only** signal for temporary role grants from the JIT access request workflow.

> **Live API verification (2026-02-27):**
> - `CreateAccessRequestV2()` fires `access_request.create` with `state=PENDING`, `user=baton-evt-test-user`, `roles=[editor]`.
> - Reviewer approval fires `access_request.review` with `state=APPROVED`, `RequestID=019c9f92-...`, but `User=""` and `Roles=[]`.

---

### Lock Events

#### `lock.created`

| Field | Value |
|-------|-------|
| Teleport event string | `lock.created` |
| Go type | `*events.LockCreate` |
| When fired | A Teleport lock is created, suspending a user |
| C1 events emitted | 1 `ResourceChangeEvent` (user) |
| Guard | Skipped if lock target has no `User` set |

**Behavior:**
- Only **user** targets are handled: `Get()` on a user surfaces `IsLocked` as `STATUS_DISABLED`.
- Role targets are ignored because the role resource has no lock/status field — `Get()` returns nothing new.
- Locks targeting nodes, logins, MFA devices, etc. are silently ignored (we don't model those).

#### `lock.deleted`

| Field | Value |
|-------|-------|
| Teleport event string | `lock.deleted` |
| Go type | `*events.LockDelete` |
| When fired | A Teleport lock is **removed**, re-enabling the user |
| C1 events emitted | 1 `ResourceChangeEvent` (user) |
| Guard | Skipped if lock target has no `User` set |

**Behavior:**
- Same logic as `lock.created` — only user targets are handled.
- After the lock is removed, `Get()` returns the user with `IsLocked=false` → `STATUS_ENABLED`.
- Unlike other delete events (e.g. `user.delete`), `lock.deleted` is safe to handle because it deletes the **lock**, not the user. The user still exists and `Get()` returns them successfully.

---

## Events NOT Handled

The following Teleport event types exist in the Go SDK but are **intentionally not handled** by this connector:

### Delete Events (Design Decision)

Resource delete events (`user.delete`, `role.deleted`, `app.delete`, `db.delete`, `access_request.delete`) are **intentionally excluded** from the event feed.

**Reason:** `ResourceChangeEvent` triggers C1 to call the resource's `Get()` method to resync. For deleted resources, `Get()` returns "not found" which C1 cannot distinguish from a transient error. Deletions are safely reconciled during the next full sync cycle instead.

**Exception:** `lock.deleted` IS handled because it deletes the *lock*, not the user — see Lock Events above.

### Access Request Submission

`access_request.create` (submission, `PENDING` state) is **intentionally excluded**. At submission time no access has been granted yet — the request is just pending. There is nothing new for C1 to discover via resync.

### App and Database Events

`app.create`, `app.update`, `db.create`, and `db.update` are **intentionally excluded** from the event feed.

**Reason:** `List()` uses `Metadata.Revision` as the resource ID for apps and databases, but audit events only carry the resource **name** — C1 cannot correlate the event to an existing resource. Apps and databases are reconciled during the next full sync. A follow-up ticket will evaluate switching to `Name` as the stable resource ID, which would unblock incremental sync for these resource types.

| Go Type | Teleport event string |
|---------|----------------------|
| `*events.AppCreate` | `app.create` |
| `*events.AppUpdate` | `app.update` |
| `*events.DatabaseCreate` | `db.create` |
| `*events.DatabaseUpdate` | `db.update` |

### Other Excluded Events

| Go Type | Reason |
|---------|--------|
| `*events.SessionStart` | Session events are not relevant for resource sync. |
| `*events.SessionEnd` | Session events are not relevant for resource sync. |
| `*events.Exec` | Command execution events are not relevant for resource sync. |

---

## Event-to-ConductorOne Mapping Summary

| Teleport Event | C1 ResourceChangeEvent | C1 CreateGrantEvent | C1 CreateRevokeEvent | Notes |
|---------------|----------------------|-------------------|---------------------|-------|
| `user.create` | User | Per role in `Roles[]` | — | Initial user creation |
| `user.update` | User | Per role in `Roles[]` | — | User modification — role **removals** are not emitted; reconciled by full sync ([details](#userupdate)) |
| `role.created` | Role | — | — | Role creation |
| `role.updated` | Role | — | — | Role modification (code T9002I) |
| ~~`app.create`~~ | — | — | — | **Not handled** — ID mismatch (Revision vs Name) |
| ~~`app.update`~~ | — | — | — | **Not handled** — ID mismatch (Revision vs Name) |
| ~~`db.create`~~ | — | — | — | **Not handled** — ID mismatch (Revision vs Name) |
| ~~`db.update`~~ | — | — | — | **Not handled** — ID mismatch (Revision vs Name) |
| `access_request.review` | — | Per role (if APPROVED) | — | Resolves original request by ID |
| `access_request.update` | — | Per role (if APPROVED) | — | State transition |
| `access_request.expire` | — | — | Per role (if EXPIRED) | Temporary access revoked |
| `lock.created` | User | — | — | User locked → `STATUS_DISABLED` |
| `lock.deleted` | User | — | — | User unlocked → `STATUS_ENABLED` |
| `user.login` | — | — | — | Emitted as `UsageEvent` (separate feed) |
| ~~`*.delete`~~ | — | — | — | **Not handled** — deletions reconciled by full sync |
| ~~`access_request.create`~~ | — | — | — | **Not handled** — request is just PENDING |
