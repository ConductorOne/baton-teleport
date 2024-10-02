package connector

import (
	"context"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/gravitational/teleport/api/types"

	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-teleport/pkg/client"
)

const appMembership = "member"

type appBuilder struct {
	resourceType *v2.ResourceType
	client       *client.TeleportClient
}

func (a *appBuilder) ResourceType(_ context.Context) *v2.ResourceType {
	return a.resourceType
}

// Create a new connector resource for a Teleport node.
func getAppResource(app types.Application) (*v2.Resource, error) {
	return rs.NewRoleResource(
		app.GetName(),
		appResourceType,
		app.GetMetadata().ID,
		[]rs.RoleTraitOption{
			rs.WithRoleProfile(
				map[string]interface{}{
					"app_id":   app.GetMetadata().ID,
					"app_name": app.GetName(),
				},
			),
		},
	)
}

// List returns all the apps from the database as resource objects.
// Apps include a NodeTrait because they are the 'shape' of a standard node.
func (a *appBuilder) List(ctx context.Context, parentId *v2.ResourceId, token *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var rv []*v2.Resource
	apps, err := a.client.GetApps(ctx)
	if err != nil {
		return nil, "", nil, err
	}

	for _, app := range apps {
		appCopy := app
		rr, err := getAppResource(appCopy)
		if err != nil {
			return nil, "", nil, err
		}
		rv = append(rv, rr)
	}

	return rv, "", nil, nil
}

func (a *appBuilder) Entitlements(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return []*v2.Entitlement{
		ent.NewAssignmentEntitlement(
			resource,
			appMembership,
			ent.WithGrantableTo(userResourceType),
			ent.WithDisplayName(fmt.Sprintf("%s App %s", resource.DisplayName, appMembership)),
			ent.WithDescription(fmt.Sprintf("Member of %s Teleport app", resource.DisplayName)),
		),
	}, "", nil, nil
}

func (a *appBuilder) Grants(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func (a *appBuilder) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) (annotations.Annotations, error) {
	return nil, nil
}

func (a *appBuilder) Revoke(ctx context.Context, grant *v2.Grant) (annotations.Annotations, error) {
	return nil, nil
}

func newAppBuilder(c *client.TeleportClient) *appBuilder {
	return &appBuilder{
		resourceType: appResourceType,
		client:       c,
	}
}
