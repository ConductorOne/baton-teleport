package connector

import (
	"fmt"

	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/gravitational/teleport/api/types"
)

// Populate entitlement options for teleport resource.
func PopulateOptions(displayName, permission, resource string) []ent.EntitlementOption {
	options := []ent.EntitlementOption{
		ent.WithDisplayName(fmt.Sprintf("%s Role %s", displayName, permission)),
		ent.WithDescription(fmt.Sprintf("%s of Teleport %s %s", permission, displayName, resource)),
		ent.WithGrantableTo(roleResourceType, userResourceType),
	}
	return options
}

func getRoleName(roleId int64, roles []types.Role) string {
	var roleName string = ""
	for _, role := range roles {
		if role.GetMetadata().ID != roleId {
			continue
		}
		roleName = role.GetMetadata().Name
	}

	return roleName
}
