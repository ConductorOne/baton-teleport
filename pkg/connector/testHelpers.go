package connector

import (
	"errors"
	"fmt"
	"os"
	"testing"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/gravitational/teleport/api/types"
	"github.com/stretchr/testify/require"
)

func CheckFileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !errors.Is(err, os.ErrNotExist)
}

func GetEntitlementForTesting(
	resource *v2.Resource,
	resourceDisplayName string,
	roleEntitlement string,
) *v2.Entitlement {
	return ent.NewAssignmentEntitlement(
		resource,
		roleEntitlement,
		ent.WithGrantableTo(roleResourceType),
		ent.WithDisplayName(fmt.Sprintf("%s Role %s", resourceDisplayName, roleEntitlement)),
		ent.WithDescription(fmt.Sprintf("%s of %s Teleport role", roleEntitlement, resourceDisplayName)),
	)
}

func GetUserResourceForTesting(
	t *testing.T,
	name string,
	description string,
) *v2.Resource {
	principal, err := userResource(
		&v2.ResourceId{},
		&types.UserV2{
			Kind:    "user",
			Version: "V7",
			Metadata: types.Metadata{
				Name:        name,
				Namespace:   "default",
				Description: description,
			},
		},
	)
	require.Nil(t, err)
	return principal
}

func GetRoleResourceForTesting(
	t *testing.T,
	name string,
	description string,
) *v2.Resource {
	resource, err := getRoleResource(
		&types.RoleV6{
			Kind:    "role",
			Version: "V7",
			Metadata: types.Metadata{
				Name:        name,
				Namespace:   "default",
				Description: description,
			},
		},
	)
	require.Nil(t, err)
	return resource
}
