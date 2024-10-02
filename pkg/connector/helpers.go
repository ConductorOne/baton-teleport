package connector

import (
	"fmt"

	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
)

// PopulateOptions - Populate entitlement options for teleport resource.
func PopulateOptions(displayName, permission, resource string) []ent.EntitlementOption {
	options := []ent.EntitlementOption{
		ent.WithDisplayName(fmt.Sprintf("%s Role %s", displayName, permission)),
		ent.WithDescription(fmt.Sprintf("%s of Teleport %s %s", permission, displayName, resource)),
		ent.WithGrantableTo(roleResourceType, userResourceType),
	}
	return options
}
