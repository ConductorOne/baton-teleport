package connector

import (
	"context"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-teleport/pkg/client"
)

type roleResourceType struct {
	resourceType *v2.ResourceType
	client       *client.TeleportClient
}

type Role struct {
	Name string
	Id   string
}

var mapRoles = make(map[string]Role)

func (r *roleResourceType) ResourceType(_ context.Context) *v2.ResourceType {
	return r.resourceType
}

// Create a new connector resource for a Datadog role.
func getRoleResource(role *Role) (*v2.Resource, error) {
	profile := map[string]interface{}{
		"role_name": role.Name,
		"role_id":   role.Id,
	}

	roleTraitOptions := []rs.RoleTraitOption{
		rs.WithRoleProfile(profile),
	}

	ret, err := rs.NewRoleResource(
		role.Name,
		resourceTypeRole,
		role.Id,
		roleTraitOptions,
	)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

// List returns all the roles from the database as resource objects.
// Roles include a RoleTrait because they are the 'shape' of a standard group.
func (r *roleResourceType) List(ctx context.Context, parentId *v2.ResourceId, token *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var rv []*v2.Resource

	roles, err := r.client.GetRoles(ctx)
	if err != nil {
		return nil, "", nil, err
	}

	for _, role := range roles {
		mapRoles[role.GetName()] = Role{
			Name: role.GetName(),
			Id:   role.GetName(),
		}
	}

	for _, role := range mapRoles {
		roleCopy := role
		rr, err := getRoleResource(&roleCopy)
		if err != nil {
			return nil, "", nil, err
		}
		rv = append(rv, rr)
	}

	return rv, "", nil, nil
}

func (r *roleResourceType) Entitlements(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func (r *roleResourceType) Grants(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func (r *roleResourceType) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) (annotations.Annotations, error) {
	return nil, nil
}

func (r *roleResourceType) Revoke(ctx context.Context, grant *v2.Grant) (annotations.Annotations, error) {
	return nil, nil
}

func newRoleBuilder(c *client.TeleportClient) *roleResourceType {
	return &roleResourceType{
		resourceType: resourceTypeRole,
		client:       c,
	}
}
