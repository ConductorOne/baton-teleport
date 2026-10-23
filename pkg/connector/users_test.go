package connector

import (
	"testing"
	"time"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/gravitational/teleport/api/types"
	"github.com/stretchr/testify/require"
)

func TestUserResourceStatusAndProfile(t *testing.T) {
	parent := &v2.ResourceId{ResourceType: "parent", Resource: "p"}

	tests := []struct {
		name   string
		locked bool
		want   v2.Status_ResourceStatus
	}{
		{name: "unlocked", locked: false, want: v2.Status_RESOURCE_STATUS_ENABLED},
		{name: "locked", locked: true, want: v2.Status_RESOURCE_STATUS_DISABLED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := types.NewUser("jane-doe")
			require.NoError(t, err)
			if tt.locked {
				user.SetLocked(time.Now().Add(time.Hour), "test")
			}

			r, err := userResource(parent, user)
			require.NoError(t, err)

			require.Equal(t, tt.want, rs.GetStatus(r).GetStatus())

			profile := rs.GetProfile(r).AsMap()
			require.Equal(t, "jane-doe", profile["name"])
			require.Equal(t, "jane", profile["first_name"])
			require.Equal(t, "doe", profile["last_name"])
			require.Equal(t, "jane-doe", profile["email"])

			ut, err := rs.GetUserTrait(r)
			require.NoError(t, err)
			require.Equal(t, "jane-doe", ut.GetLogin())
			require.Equal(t, v2.UserTrait_ACCOUNT_TYPE_HUMAN, ut.GetAccountType())
			require.Equal(t, "jane-doe", ut.GetEmails()[0].GetAddress())
		})
	}
}
