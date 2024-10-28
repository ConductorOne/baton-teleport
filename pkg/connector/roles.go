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
	userCache    []types.User
}

func (r *roleBuilder) ResourceType(_ context.Context) *v2.ResourceType {
	return r.resourceType
}

// Create a new connector resource for a Teleport role.
func getRoleResource(role types.Role) (*v2.Resource, error) {
	roleName := role.GetMetadata().Name
	return rs.NewRoleResource(
		role.GetName(),
		roleResourceType,
		roleName,
		[]rs.RoleTraitOption{
			rs.WithRoleProfile(
				map[string]interface{}{
					"role_id":          role.GetMetadata().Revision,
					"role_name":        roleName,
					"role_description": role.GetMetadata().Description,
				},
			),
		},
	)
}

func (r *roleBuilder) GetUsers(ctx context.Context) ([]types.User, error) {
	if len(r.userCache) != 0 {
		return r.userCache, nil
	}

	users, err := r.client.GetUsers(ctx, false)
	if err != nil {
		return []types.User{}, err
	}

	r.userCache = users
	return users, nil
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
		roleCopy := role
		rr, err := getRoleResource(roleCopy)
		if err != nil {
			return nil, "", nil, err
		}
		rv = append(rv, rr)
	}

	// clear the cache
	r.userCache = []types.User{}
	return rv, "", nil, nil
}

func (r *roleBuilder) Entitlements(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return []*v2.Entitlement{
		ent.NewAssignmentEntitlement(
			resource,
			roleMembership,
			ent.WithGrantableTo(userResourceType),
			ent.WithDisplayName(fmt.Sprintf("%s Role %s", resource.DisplayName, roleMembership)),
			ent.WithDescription(fmt.Sprintf("Member of %s Teleport role", resource.DisplayName)),
		),
	}, "", nil, nil
}

func (r *roleBuilder) Grants(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	var rv []*v2.Grant
	users, err := r.GetUsers(ctx)
	if err != nil {
		return nil, "", nil, err
	}

	for _, user := range users {
		userCopy := user
		ur, err := userResource(resource.Id, userCopy)
		if err != nil {
			return nil, "", nil, fmt.Errorf("error creating user resource for role %s: %w", resource.Id.Resource, err)
		}

		for _, role := range user.GetRoles() {
			if role != resource.DisplayName {
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

	user, err := r.client.GetUser(ctx, userName, false)
	if err != nil {
		return nil, err
	}

	user.SetLogins(append(user.GetLogins(), userName))
	user.AddRole(prodRole.GetName())
	updatedUser, err := r.client.UpdateUser(ctx, user.(*types.UserV2))
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
	user, err := r.client.GetUser(ctx, userName, false)
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
	updatedUser, err := r.client.UpdateUser(ctx, user.(*types.UserV2))
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
