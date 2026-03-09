package main

import (
	"context"
	"fmt"
	"os"
	"time"

	teleport "github.com/gravitational/teleport/api/client"
	apidefaults "github.com/gravitational/teleport/api/defaults"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/events"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	creds := teleport.LoadIdentityFile("auth.pem")
	client, err := teleport.New(ctx, teleport.Config{
		Addrs:       []string{"horizon.teleport.sh:443"},
		Credentials: []teleport.Credentials{creds},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		return
	}
	defer client.Close()

	now := time.Now().UTC()
	from := now.Add(-2 * time.Hour)

	fmt.Fprintln(os.Stdout, "=== USAGE EVENTS (user.login - last 7 days) ===")
	loginResult, _, err := client.SearchEvents(ctx, now.Add(-7*24*time.Hour), now, apidefaults.Namespace,
		[]string{"user.login"}, 50, types.EventOrderDescending, "", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to search login events: %v\n", err)
	} else {
		userLogins := make(map[string]string)
		for _, evt := range loginResult {
			ul, ok := evt.(*events.UserLogin)
			if !ok {
				continue
			}
			fmt.Fprintf(os.Stdout, "  user=%-45s success=%-5t time=%s\n", ul.User, ul.Success, ul.GetTime().Format(time.RFC3339))
			if ul.Success {
				if _, exists := userLogins[ul.User]; !exists {
					userLogins[ul.User] = ul.GetTime().Format(time.RFC3339)
				}
			}
		}
		fmt.Fprintln(os.Stdout, "\n--- Last login per user ---")
		for user, lastLogin := range userLogins {
			fmt.Fprintf(os.Stdout, "  %-45s %s\n", user, lastLogin)
		}
	}

	fmt.Fprintln(os.Stdout, "\n=== AUDIT EVENTS (last 2 hours) ===")
	auditTypes := []string{
		"user.create", "user.update", "user.delete",
		"role.created", "role.updated", "role.deleted",
		"app.create", "app.update", "app.delete",
		"db.create", "db.update", "db.delete",
		"access_request.create", "access_request.review",
		"access_request.update", "access_request.expire",
		"access_request.delete",
		"lock.created", "lock.deleted",
	}
	auditResult, _, err := client.SearchEvents(ctx, from, now, apidefaults.Namespace,
		auditTypes, 100, types.EventOrderAscending, "", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to search audit events: %v\n", err)
		return
	}

	if len(auditResult) == 0 {
		fmt.Fprintln(os.Stdout, "  (no audit events found in last 2 hours)")
	}

	for _, evt := range auditResult {
		t := evt.GetTime().Format(time.RFC3339)
		id := evt.GetID()

		switch e := evt.(type) {
		case *events.UserCreate:
			fmt.Fprintf(os.Stdout, "  [%s] user.create      id=%-36s user=%-40s roles=%v\n", t, id, e.Name, e.Roles)
		case *events.UserUpdate:
			fmt.Fprintf(os.Stdout, "  [%s] user.update      id=%-36s user=%-40s roles=%v\n", t, id, e.Name, e.Roles)
		case *events.UserDelete:
			fmt.Fprintf(os.Stdout, "  [%s] user.delete      id=%-36s user=%s\n", t, id, e.Name)
		case *events.RoleCreate:
			fmt.Fprintf(os.Stdout, "  [%s] role.create      id=%-36s role=%s\n", t, id, e.Name)
		case *events.RoleUpdate:
			fmt.Fprintf(os.Stdout, "  [%s] role.update      id=%-36s role=%s\n", t, id, e.Name)
		case *events.RoleDelete:
			fmt.Fprintf(os.Stdout, "  [%s] role.delete      id=%-36s role=%s\n", t, id, e.Name)
		case *events.AppCreate:
			fmt.Fprintf(os.Stdout, "  [%s] app.create       id=%-36s app=%s\n", t, id, e.Name)
		case *events.AppUpdate:
			fmt.Fprintf(os.Stdout, "  [%s] app.update       id=%-36s app=%s\n", t, id, e.Name)
		case *events.AppDelete:
			fmt.Fprintf(os.Stdout, "  [%s] app.delete       id=%-36s app=%s\n", t, id, e.Name)
		case *events.DatabaseCreate:
			fmt.Fprintf(os.Stdout, "  [%s] db.create        id=%-36s db=%s\n", t, id, e.Name)
		case *events.DatabaseUpdate:
			fmt.Fprintf(os.Stdout, "  [%s] db.update        id=%-36s db=%s\n", t, id, e.Name)
		case *events.DatabaseDelete:
			fmt.Fprintf(os.Stdout, "  [%s] db.delete        id=%-36s db=%s\n", t, id, e.Name)
		case *events.AccessRequestCreate:
			fmt.Fprintf(os.Stdout, "  [%s] access_req.create id=%-36s user=%-30s roles=%v state=%s\n",
				t, id, e.User, e.Roles, e.RequestState)
		case *events.AccessRequestDelete:
			fmt.Fprintf(os.Stdout, "  [%s] access_req.delete id=%-36s user=%s requestID=%s\n",
				t, id, e.User, e.RequestID)
		case *events.LockCreate:
			target := e.Lock.Target
			fmt.Fprintf(os.Stdout, "  [%s] lock.create      id=%-36s target=user:%-20s role:%s\n",
				t, id, target.User, target.Role)
		case *events.LockDelete:
			target := e.Lock.Target
			fmt.Fprintf(os.Stdout, "  [%s] lock.delete      id=%-36s target=user:%-20s role:%s\n",
				t, id, target.User, target.Role)
		default:
			fmt.Fprintf(os.Stdout, "  [%s] %-16s id=%s\n", t, evt.GetType(), id)
		}
	}

	fmt.Fprintf(os.Stdout, "\nTotal audit events: %d\n", len(auditResult))
}
