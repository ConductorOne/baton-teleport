// populate-test-events creates test resources in a live Teleport instance
// to generate audit events for all resource types the event feed handles.
//
// It creates, modifies, and cleans up resources in sequence, pausing between
// operations so each gets its own audit log entry. After running this tool,
// use cmd/test-events to verify all expected audit events appeared.
//
// Usage:
//
//	go run ./cmd/populate-test-events
//
// Requires auth.pem in the working directory.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	teleport "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
)

const (
	proxyAddress = "horizon.teleport.sh:443"
	identityFile = "auth.pem"

	testRoleName = "baton-evt-test-role"
	testUserName = "baton-evt-test-user"
	testLockName = "baton-evt-test-lock"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	fmt.Fprintln(os.Stdout, "=== Populate Test Events ===")
	fmt.Fprintf(os.Stdout, "Target: %s\n\n", proxyAddress)

	creds := teleport.LoadIdentityFile(identityFile)
	client, err := teleport.New(ctx, teleport.Config{
		Addrs:       []string{proxyAddress},
		Credentials: []teleport.Credentials{creds},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		return
	}
	defer client.Close()

	fmt.Fprintln(os.Stdout, "Connected successfully.")
	fmt.Fprintln(os.Stdout)

	fmt.Fprintln(os.Stdout, "--- Phase 1: Cleanup leftovers ---")
	cleanupAll(ctx, client)
	pause("cleanup → creation")

	fmt.Fprintln(os.Stdout, "--- Phase 2: Create resources ---")

	fmt.Fprint(os.Stdout, "  Creating role... ")
	role, err := types.NewRole(testRoleName, types.RoleSpecV6{
		Allow: types.RoleConditions{
			Logins: []string{"test-login"},
			Request: &types.AccessRequestConditions{
				Roles: []string{"editor"},
			},
		},
	})
	if err != nil {
		fatalf("create role spec: %v", err)
	}
	_, err = client.CreateRole(ctx, role)
	if err != nil {
		fatalf("CreateRole: %v", err)
	}
	fmt.Fprintln(os.Stdout, "OK → expect role.created event")
	pause("role create → user create")

	fmt.Fprint(os.Stdout, "  Creating user... ")
	user, err := types.NewUser(testUserName)
	if err != nil {
		fatalf("create user spec: %v", err)
	}
	user.SetRoles([]string{"access", testRoleName})
	_, err = client.CreateUser(ctx, user)
	if err != nil {
		fatalf("CreateUser: %v", err)
	}
	fmt.Fprintln(os.Stdout, "OK → expect user.create event with roles=[access, "+testRoleName+"]")
	pause("user create → role update")

	fmt.Fprintln(os.Stdout, "\n--- Phase 3: Modify resources ---")

	fmt.Fprint(os.Stdout, "  Updating role... ")
	existingRole, err := client.GetRole(ctx, testRoleName)
	if err != nil {
		fatalf("GetRole: %v", err)
	}
	existingRole.SetLogins(types.Allow, []string{"test-login", "updated-login"})
	_, err = client.UpsertRole(ctx, existingRole)
	if err != nil {
		fatalf("UpsertRole: %v", err)
	}
	fmt.Fprintln(os.Stdout, "OK → expect role.created or role.updated event")
	pause("role update → user update")

	fmt.Fprint(os.Stdout, "  Updating user roles... ")
	existingUser, err := client.GetUser(ctx, testUserName, false)
	if err != nil {
		fatalf("GetUser: %v", err)
	}
	existingUser.SetRoles([]string{"access", testRoleName, "editor"})
	_, err = client.UpdateUser(ctx, existingUser)
	if err != nil {
		fatalf("UpdateUser: %v", err)
	}
	fmt.Fprintln(os.Stdout, "OK → expect user.update event with roles=[access, "+testRoleName+", editor]")
	pause("user update → lock create")

	fmt.Fprintln(os.Stdout, "\n--- Phase 4: Lock operations ---")

	fmt.Fprint(os.Stdout, "  Creating lock... ")
	lock, err := types.NewLock(testLockName, types.LockSpecV2{
		Target: types.LockTarget{
			User: testUserName,
		},
		Message: "baton event feed test lock",
	})
	if err != nil {
		fatalf("create lock spec: %v", err)
	}
	err = client.UpsertLock(ctx, lock)
	if err != nil {
		fatalf("UpsertLock: %v", err)
	}
	fmt.Fprintln(os.Stdout, "OK → expect lock.created event with target.user="+testUserName)
	pause("lock create → access request")

	fmt.Fprintln(os.Stdout, "\n--- Phase 5: Access request ---")
	fmt.Fprint(os.Stdout, "  Creating access request... ")
	accessReqID := uuid.New().String()
	accessReq, err := types.NewAccessRequest(accessReqID, testUserName, "editor")
	if err != nil {
		fmt.Fprintf(os.Stdout, "SKIP (cannot create spec: %v)\n", err)
	} else {
		accessReq.SetRequestReason("baton event feed test")
		_, err = client.CreateAccessRequestV2(ctx, accessReq)
		if err != nil {
			fmt.Fprintf(os.Stdout, "SKIP (create failed: %v)\n", err)
			fmt.Fprintln(os.Stdout, "  (Access requests may require specific role configuration)")
		} else {
			fmt.Fprintln(os.Stdout, "OK → expect access_request.create event (PENDING)")
			pause("access request create → approve")

			fmt.Fprint(os.Stdout, "  Approving access request... ")
			err = client.SetAccessRequestState(ctx, types.AccessRequestUpdate{
				RequestID: accessReq.GetName(),
				State:     types.RequestState_APPROVED,
				Reason:    "baton event feed test approval",
			})
			if err != nil {
				fmt.Fprintf(os.Stdout, "SKIP (approve failed: %v)\n", err)
			} else {
				fmt.Fprintln(os.Stdout, "OK → expect access_request.review/update event (APPROVED)")
			}
		}
	}
	pause("access request → delete lock")

	fmt.Fprintln(os.Stdout, "\n--- Phase 6: Delete resources (for verification of exclusion) ---")

	fmt.Fprint(os.Stdout, "  Deleting lock... ")
	err = client.DeleteLock(ctx, testLockName)
	if err != nil {
		fmt.Fprintf(os.Stdout, "WARN: %v\n", err)
	} else {
		fmt.Fprintln(os.Stdout, "OK → expect lock.deleted event (handled — user re-enabled)")
	}
	pause("lock delete → user delete")

	fmt.Fprint(os.Stdout, "  Deleting user... ")
	err = client.DeleteUser(ctx, testUserName)
	if err != nil {
		fmt.Fprintf(os.Stdout, "WARN: %v\n", err)
	} else {
		fmt.Fprintln(os.Stdout, "OK → expect user.delete event (should be IGNORED by feed)")
	}
	pause("user delete → role delete")

	fmt.Fprint(os.Stdout, "  Deleting role... ")
	err = client.DeleteRole(ctx, testRoleName)
	if err != nil {
		fmt.Fprintf(os.Stdout, "WARN: %v\n", err)
	} else {
		fmt.Fprintln(os.Stdout, "OK → expect role.deleted event (should be IGNORED by feed)")
	}

	fmt.Fprintln(os.Stdout, "\n=== Done ===")
	fmt.Fprintln(os.Stdout, "Now run: go run ./cmd/test-events")
	fmt.Fprintln(os.Stdout, "to verify all expected audit events appeared.")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Expected events (handled by audit feed):")
	fmt.Fprintln(os.Stdout, "  1. role.created              → "+testRoleName+" (initial create)")
	fmt.Fprintln(os.Stdout, "  2. user.create               → "+testUserName+" (roles=[access, "+testRoleName+"])")
	fmt.Fprintln(os.Stdout, "  3. role.created/role.updated  → "+testRoleName+" (upsert modification)")
	fmt.Fprintln(os.Stdout, "  4. user.update               → "+testUserName+" (roles=[access, "+testRoleName+", editor])")
	fmt.Fprintln(os.Stdout, "  5. lock.created              → target.user="+testUserName+" (user disabled)")
	fmt.Fprintln(os.Stdout, "  6. access_request.review/update → APPROVED (if supported)")
	fmt.Fprintln(os.Stdout, "  7. lock.deleted              → target.user="+testUserName+" (user re-enabled)")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Expected events (excluded from feed — reconciled by full sync):")
	fmt.Fprintln(os.Stdout, "  8. access_request.create → PENDING (no access granted yet)")
	fmt.Fprintln(os.Stdout, "  9. user.delete           → "+testUserName)
	fmt.Fprintln(os.Stdout, "  10. role.deleted         → "+testRoleName)
}

func cleanupAll(ctx context.Context, client *teleport.Client) {
	requests, err := client.GetAccessRequests(ctx, types.AccessRequestFilter{})
	if err == nil {
		for _, req := range requests {
			if req.GetUser() == testUserName {
				_ = client.DeleteAccessRequest(ctx, req.GetName())
				fmt.Fprintf(os.Stdout, "  Cleaned up access request: %s\n", req.GetName())
			}
		}
	}

	if err := client.DeleteLock(ctx, testLockName); err == nil {
		fmt.Fprintf(os.Stdout, "  Cleaned up lock: %s\n", testLockName)
	}

	if err := client.DeleteUser(ctx, testUserName); err == nil {
		fmt.Fprintf(os.Stdout, "  Cleaned up user: %s\n", testUserName)
	}

	if err := client.DeleteRole(ctx, testRoleName); err == nil {
		fmt.Fprintf(os.Stdout, "  Cleaned up role: %s\n", testRoleName)
	}
}

func pause(label string) {
	fmt.Fprintf(os.Stdout, "  (waiting 3s: %s)\n", label)
	time.Sleep(3 * time.Second)
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
