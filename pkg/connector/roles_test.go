package connector

import (
	"context"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
	"github.com/conductorone/baton-teleport/pkg/client"
	"github.com/stretchr/testify/require"
)

var (
	proxyAddrTest   = "conductorone-c1.teleport.sh:443"
	filePathTest    = "../../auth.pem"
	roleName        = "reviewer"
	roleDescription = "Review access requests"
	roleEntitlement = "member"
	userName        = "miguel_chavez_m@hotmail.com"
	userDescription = "user for testing"
)

func TestRoles(t *testing.T) {
	ctx := context.Background()
	if !CheckFileExists(filePathTest) {
		t.Skip()
	}

	cliTest, err := client.New(ctx, proxyAddrTest, filePathTest, "")
	require.Nil(t, err)

	r := &roleBuilder{
		resourceType: roleResourceType,
		client:       cliTest,
	}

	principal := GetUserResourceForTesting(t, userName, userDescription)
	resource := GetRoleResourceForTesting(t, roleName, roleDescription)

	t.Run("role builder should fetch a list of users", func(t *testing.T) {
		res, _, _, err := r.List(ctx, &v2.ResourceId{}, &pagination.Token{})
		require.Nil(t, err)
		require.NotNil(t, res)
	})

	t.Run("role builder revoke a role", func(t *testing.T) {
		gr := grant.NewGrant(resource, roleEntitlement, principal.Id)
		require.NotNil(t, gr)

		// --revoke-grant "role:reviewer:member:user:miguel_chavez_m@hotmail.com"
		_, err = r.Revoke(ctx, gr)
		require.Nil(t, err)
	})

	t.Run("role builder grant a role", func(t *testing.T) {
		entitlement := GetEntitlementForTesting(resource, roleName, roleEntitlement)
		require.NotNil(t, entitlement)

		// --grant-entitlement "role:reviewer:member" \
		//   --grant-principal-type user \
		//   --grant-principal "miguel_chavez_m@hotmail.com"
		_, err = r.Grant(ctx, principal, entitlement)
		require.Nil(t, err)
	})
}
