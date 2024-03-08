package connector

import (
	"context"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/helpers"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-teleport/pkg/client"
	"github.com/gravitational/teleport/api/types"
)

type userBuilder struct {
	resourceType *v2.ResourceType
	client       *client.TeleportClient
}

var mapUsers = make(map[string]User)

type User struct {
	Name   string
	Email  string
	Kind   string
	Roles  []string
	Traits map[string][]string
}

func (o *userBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return userResourceType
}

func userResource(ctx context.Context, pId *v2.ResourceId, user *User) (*v2.Resource, error) {
	firstName, lastName := helpers.SplitFullName(user.Name)
	profile := map[string]interface{}{
		"name":       user.Name,
		"email":      user.Email,
		"user_id":    user.Email,
		"first_name": firstName,
		"last_name":  lastName,
	}

	resource, err := resource.NewUserResource(
		user.Email,
		userResourceType,
		user.Email,
		[]resource.UserTraitOption{
			resource.WithUserProfile(profile),
			resource.WithEmail(user.Email, true),
			resource.WithUserLogin(user.Email),
			resource.WithStatus(v2.UserTrait_Status_STATUS_ENABLED),
		},
		resource.WithParentResourceID(pId),
	)

	if err != nil {
		return nil, err
	}

	return resource, nil
}

func addUsers(users []types.User) {
	for _, user := range users {
		mapUsers[user.GetName()] = User{
			Name:   user.GetName(),
			Email:  user.GetName(),
			Kind:   user.GetKind(),
			Roles:  user.GetRoles(),
			Traits: user.GetTraits(),
		}
	}
}

// List returns all the users from the database as resource objects.
// Users include a UserTrait because they are the 'shape' of a standard user.
func (u *userBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var rv []*v2.Resource
	if len(mapUsers) == 0 {
		users, err := u.client.GetUsers(ctx)
		if err != nil {
			return nil, "", nil, err
		}

		addUsers(users)
	}

	for _, userEntry := range mapUsers {
		userEntryCopy := userEntry

		ur, err := userResource(ctx, parentResourceID, &userEntryCopy)
		if err != nil {
			return nil, "", nil, err
		}

		rv = append(rv, ur)
	}

	return rv, "", nil, nil
}

// Entitlements always returns an empty slice for users.
func (o *userBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

// Grants always returns an empty slice for users since they don't have any entitlements.
func (o *userBuilder) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func newUserBuilder(c *client.TeleportClient) *userBuilder {
	return &userBuilder{
		resourceType: userResourceType,
		client:       c,
	}
}
