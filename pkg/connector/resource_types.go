package connector

import (
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
)

// The user resource type is for all user objects from the database.
var (
	userResourceType = &v2.ResourceType{
		Id:          "user",
		DisplayName: "User",
		Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_USER},
		Annotations: annotations.New(&v2.SkipEntitlementsAndGrants{}),
	}
	roleResourceType = &v2.ResourceType{
		Id:          "role",
		DisplayName: "Role",
		Traits: []v2.ResourceType_Trait{
			v2.ResourceType_TRAIT_ROLE,
		},
	}
	nodeResourceType = &v2.ResourceType{
		Id:          "node",
		DisplayName: "Node",
	}
	appResourceType = &v2.ResourceType{
		Id:          "app",
		DisplayName: "App",
	}
	dbResourceType = &v2.ResourceType{
		Id:          "database",
		DisplayName: "Database",
	}
)
