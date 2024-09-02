package connector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
	"github.com/conductorone/baton-teleport/pkg/client"
	teleport "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

var (
	ctxTest       = context.Background()
	proxyAddrTest = "conductorone-c1.teleport.sh:443"
	filePathTest  = "../../auth.pem"
)

func Test_roleBuilderList(t *testing.T) {
	if !checkFileExists(filePathTest) {
		t.Skip()
	}

	cliTest, err := getTestingClient(ctxTest, proxyAddrTest, filePathTest)
	require.Nil(t, err)

	r := &roleBuilder{
		resourceType: &v2.ResourceType{},
		client:       cliTest,
	}
	res, _, _, err := r.List(ctxTest, &v2.ResourceId{}, &pagination.Token{})
	require.Nil(t, err)
	require.NotNil(t, res)
}

func getTestingClient(ctx context.Context, proxyAddr, filePath string) (*client.TeleportClient, error) {
	tc, err := NewTestingClient(ctx, proxyAddr, filePath)
	if err != nil {
		return nil, err
	}

	return tc, nil
}

func NewTestingClient(ctx context.Context, proxyAddr, file string) (*client.TeleportClient, error) {
	const initTimeout = time.Duration(10) * time.Second
	tc := &client.TeleportClient{
		ProxyAddr: proxyAddr,
	}
	ctx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()

	creds := teleport.LoadIdentityFile(file)
	client, err := teleport.New(ctx, teleport.Config{
		Addrs:       []string{proxyAddr},
		Credentials: []teleport.Credentials{creds},
		DialOpts: []grpc.DialOption{
			grpc.WithReturnConnectionError(),
		},
	})
	if err != nil {
		return nil, err
	}

	return tc.SetClient(ctx, client), nil
}

func checkFileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !errors.Is(err, os.ErrNotExist)
}

func TestRoleRevoke(t *testing.T) {
	var (
		roleName        = "reviewer"
		roleDescription = "Review access requests"
		userName        = "miguel_chavez_m@hotmail.com"
		userDescription = "user for testing"
		roleEntitlement = "member"
	)
	if !checkFileExists(filePathTest) {
		t.Skip()
	}

	accUser := getUserResourceForTesting(userName, userDescription)
	ur, err := userResource(ctxTest, &v2.ResourceId{}, accUser)
	require.Nil(t, err)

	role := getRoleResourceForTesting(roleName, roleDescription)
	resource, err := getRoleResource(role)
	require.Nil(t, err)

	cliTest, err := getTestingClient(ctxTest, proxyAddrTest, filePathTest)
	require.Nil(t, err)

	roleBuilder := getRoleBuilderForTesting(cliTest)
	gr := grant.NewGrant(resource, roleEntitlement, ur.Id)
	annos := annotations.Annotations(gr.Annotations)
	gr.Annotations = annos

	// --revoke-grant "role:reviewer:member:user:miguel_chavez_m@hotmail.com"
	_, err = roleBuilder.Revoke(ctxTest, gr)
	require.Nil(t, err)
}

func TestResourceTypeGrant(t *testing.T) {
	var (
		userName        = "miguel_chavez_m@hotmail.com"
		userDescription = "User for testing"
		roleName        = "reviewer"
		roleDescription = "Review access requests"
		roleEntitlement = "member"
	)
	if !checkFileExists(filePathTest) {
		t.Skip()
	}

	accUser := getUserResourceForTesting(userName, userDescription)
	principal, err := userResource(ctxTest, &v2.ResourceId{}, accUser)
	require.Nil(t, err)

	role := getRoleResourceForTesting(roleName, roleDescription)
	resource, err := getRoleResource(role)
	require.Nil(t, err)

	entitlement := getEntitlementForTesting(resource, roleName, roleEntitlement)

	cliTest, err := getTestingClient(ctxTest, proxyAddrTest, filePathTest)
	require.Nil(t, err)

	roleBuilder := getRoleBuilderForTesting(cliTest)
	// --grant-entitlement "role:reviewer:member" --grant-principal-type user --grant-principal "miguel_chavez_m@hotmail.com"
	_, err = roleBuilder.Grant(ctxTest, principal, entitlement)
	require.Nil(t, err)
}

func getRoleBuilderForTesting(c *client.TeleportClient) *roleBuilder {
	return &roleBuilder{
		resourceType: roleResourceType,
		client:       c,
	}
}

func getEntitlementForTesting(resource *v2.Resource, resourceDisplayName, roleEntitlement string) *v2.Entitlement {
	options := []ent.EntitlementOption{
		ent.WithGrantableTo(roleResourceType),
		ent.WithDisplayName(fmt.Sprintf("%s Role %s", resourceDisplayName, roleEntitlement)),
		ent.WithDescription(fmt.Sprintf("%s of %s Teleport role", roleEntitlement, resourceDisplayName)),
	}

	return ent.NewAssignmentEntitlement(resource, roleEntitlement, options...)
}

func getUserResourceForTesting(name, description string) *types.UserV2 {
	return &types.UserV2{
		Kind:    "user",
		Version: "V7",
		Metadata: types.Metadata{
			Name:        name,
			Namespace:   "default",
			Description: description,
		},
	}
}

func getRoleResourceForTesting(name, description string) *types.RoleV6 {
	return &types.RoleV6{
		Kind:    "role",
		Version: "V7",
		Metadata: types.Metadata{
			Name:        name,
			Namespace:   "default",
			Description: description,
		},
	}
}
