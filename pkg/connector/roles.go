package connector

import (
	"context"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/gravitational/teleport/api/types"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"

	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-teleport/pkg/client"
)

const roleMembership = "member"

type roleBuilder struct {
	resourceType *v2.ResourceType
	client       *client.TeleportClient
}

type Role struct {
	Name string
	Id   string
}

var mapRoles = make(map[string]Role)

func (r *roleBuilder) ResourceType(_ context.Context) *v2.ResourceType {
	return r.resourceType
}

// Create a new connector resource for a Teleport role.
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
		roleResourceType,
		role.Id,
		roleTraitOptions,
	)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

// List returns all the roles from the database as resource objects.
// Roles include a RoleTrait because they are the 'shape' of a standard role.
func (r *roleBuilder) List(ctx context.Context, parentId *v2.ResourceId, token *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
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

func (r *roleBuilder) Entitlements(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	var rv []*v2.Entitlement
	assignmentOptions := []ent.EntitlementOption{
		ent.WithGrantableTo(userResourceType),
		ent.WithDisplayName(fmt.Sprintf("%s Role %s", resource.DisplayName, roleMembership)),
		ent.WithDescription(fmt.Sprintf("Member of %s Teleport role", resource.DisplayName)),
	}

	rv = append(rv, ent.NewAssignmentEntitlement(
		resource,
		roleMembership,
		assignmentOptions...,
	))

	return rv, "", nil, nil
}

func (r *roleBuilder) Grants(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	var rv []*v2.Grant
	if len(mapUsers) == 0 {
		users, err := r.client.GetUsers(ctx)
		if err != nil {
			return nil, "", nil, err
		}

		addUsers(users)
	}

	for _, userEntry := range mapUsers {
		userEntryCopy := userEntry
		ur, err := userResource(ctx, resource.Id, &userEntryCopy)
		if err != nil {
			return nil, "", nil, fmt.Errorf("error creating user resource for role %s: %w", resource.Id.Resource, err)
		}

		for _, role := range userEntry.Roles {
			if role != resource.Id.Resource {
				continue
			}
			gr := grant.NewGrant(resource, roleMembership, ur.Id)
			rv = append(rv, gr)
		}
	}

	return rv, "", nil, nil
}

func (r *roleBuilder) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)
	userName := principal.Id.Resource
	roleName := entitlement.Resource.Id.Resource

	if principal.Id.ResourceType != userResourceType.Id {
		l.Warn(
			"baton-teleport: only users can be granted role membership",
			zap.String("principal_type", principal.Id.ResourceType),
			zap.String("principal_id", principal.Id.Resource),
		)
		return nil, fmt.Errorf("baton-teleport: only users can be granted role membership")
	}

	// Create an MFA required role for "prod" nodes.
	prodRole, err := types.NewRole(roleName, types.RoleSpecV6{
		Options: types.RoleOptions{
			RequireMFAType: types.RequireMFAType_SESSION,
		},
		Allow: types.RoleConditions{
			Logins:     []string{userName},
			NodeLabels: types.Labels{},
		},
	})
	if err != nil {
		return nil, err
	}

	user, err := r.client.GetUser(ctx, userName)
	if err != nil {
		return nil, err
	}

	user.SetLogins(append(user.GetLogins(), userName))
	user.AddRole(prodRole.GetName())
	updatedUser, err := r.client.UpdateUserRole(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("teleport-connector: failed to add role: %s", err.Error())
	}

	l.Warn("Role Membership has been created.",
		zap.String("Name", updatedUser.GetName()),
		zap.String("Namespace", updatedUser.GetMetadata().Namespace),
		zap.Time("CreatedAt", updatedUser.GetCreatedBy().Time),
	)

	return nil, nil
}

func (r *roleBuilder) Revoke(ctx context.Context, grant *v2.Grant) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)
	var roleList []string
	entitlement := grant.Entitlement
	principal := grant.Principal

	if principal.Id.ResourceType != userResourceType.Id {
		l.Warn(
			"baton-teleport: only users can have role membership revoked",
			zap.String("principal_type", principal.Id.ResourceType),
			zap.String("principal_id", principal.Id.Resource),
		)
		return nil, fmt.Errorf("teleport-connector: only users can have role membership revoked")
	}

	roleName := entitlement.Resource.Id.Resource
	userName := principal.Id.Resource
	user, err := r.client.GetUser(ctx, userName)
	if err != nil {
		return nil, err
	}

	user.SetLogins(append(user.GetLogins(), userName))
	for _, role := range user.GetRoles() {
		if role != roleName {
			roleList = append(roleList, role)
		}
	}

	user.SetRoles(roleList)
	updatedUser, err := r.client.UpdateUserRole(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("teleport-connector: failed to revoke role: %s", err.Error())
	}

	l.Warn("Role Membership has been revoked.",
		zap.String("Name", updatedUser.GetName()),
		zap.String("Namespace", updatedUser.GetMetadata().Namespace),
		zap.Time("CreatedAt", updatedUser.GetCreatedBy().Time),
	)

	return nil, nil
}

func newRoleBuilder(c *client.TeleportClient) *roleBuilder {
	return &roleBuilder{
		resourceType: roleResourceType,
		client:       c,
	}
}
