package connector

import (
	"fmt"
	"regexp"
	"strings"

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

func cleanResourceName(name string) string {
	name = strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	re := regexp.MustCompile(`[^a-z0-9\-.\+]+`)
	return re.ReplaceAllString(name, "")
}

func splitDashSeparatedName(name string) (string, string) {
	names := strings.SplitN(name, "-", 2)
	var firstName, lastName string
	if len(names) > 0 {
		firstName = names[0]
	}
	if len(names) > 1 {
		lastName = names[1]
	}
	return firstName, lastName
}
