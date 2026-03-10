package connector

import (
	"context"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
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
	appId := app.GetMetadata().Revision
	return rs.NewRoleResource(
		app.GetName(),
		appResourceType,
		appId,
		[]rs.RoleTraitOption{
			rs.WithRoleProfile(
				map[string]interface{}{
					"app_id":   appId,
					"app_name": app.GetName(),
				},
			),
		},
	)
}

// List returns all the apps from the database as resource objects.
// Apps include a NodeTrait because they are the 'shape' of a standard node.
func (a *appBuilder) List(ctx context.Context, _ *v2.ResourceId, _ rs.SyncOpAttrs) ([]*v2.Resource, *rs.SyncOpResults, error) {
	var rv []*v2.Resource
	apps, err := a.client.GetApps(ctx)
	if err != nil {
		return nil, nil, err
	}

	for _, app := range apps {
		appCopy := app
		rr, err := getAppResource(appCopy)
		if err != nil {
			return nil, nil, err
		}
		rv = append(rv, rr)
	}

	return rv, nil, nil
}

func (a *appBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ rs.SyncOpAttrs) ([]*v2.Entitlement, *rs.SyncOpResults, error) {
	return []*v2.Entitlement{
		ent.NewAssignmentEntitlement(
			resource,
			appMembership,
			ent.WithGrantableTo(userResourceType),
			ent.WithDisplayName(fmt.Sprintf("%s App %s", resource.DisplayName, appMembership)),
			ent.WithDescription(fmt.Sprintf("Member of %s Teleport app", resource.DisplayName)),
		),
	}, nil, nil
}

func (a *appBuilder) Grants(_ context.Context, _ *v2.Resource, _ rs.SyncOpAttrs) ([]*v2.Grant, *rs.SyncOpResults, error) {
	return nil, nil, nil
}

func newAppBuilder(c *client.TeleportClient) *appBuilder {
	return &appBuilder{
		resourceType: appResourceType,
		client:       c,
	}
}
