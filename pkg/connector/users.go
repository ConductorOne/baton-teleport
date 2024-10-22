package connector

import (
	"context"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-teleport/pkg/client"
	"github.com/gravitational/teleport/api/types"
)

type userBuilder struct {
	resourceType *v2.ResourceType
	client       *client.TeleportClient
}

func (o *userBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return userResourceType
}

func userResource(pId *v2.ResourceId, user types.User) (*v2.Resource, error) {
	var (
		accountType = v2.UserTrait_ACCOUNT_TYPE_HUMAN
		status      v2.UserTrait_Status_Status
	)
	firstName, lastName := resource.SplitFullName(user.GetName())
	profile := map[string]interface{}{
		"name":       user.GetName(),
		"email":      user.GetName(),
		"user_id":    user.GetMetadata().Revision,
		"first_name": firstName,
		"last_name":  lastName,
	}

	// TODO: IsBot is false for @teleport-access-approval-bot
	if user.IsBot() {
		accountType = v2.UserTrait_ACCOUNT_TYPE_SERVICE
	}

	switch user.GetStatus().IsLocked {
	case true:
		status = v2.UserTrait_Status_STATUS_DISABLED
	case false:
		status = v2.UserTrait_Status_STATUS_ENABLED
	default:
		status = v2.UserTrait_Status_STATUS_UNSPECIFIED
	}

	return resource.NewUserResource(
		user.GetName(),
		userResourceType,
		user.GetName(),
		[]resource.UserTraitOption{
			resource.WithUserProfile(profile),
			// TODO: This is not always an email address, at least not for @teleport-access-approval-bot or bots
			resource.WithEmail(user.GetName(), true),
			resource.WithUserLogin(user.GetName()),
			resource.WithStatus(status),
			resource.WithAccountType(accountType),
		},
		resource.WithParentResourceID(pId),
	)
}

// List returns all the users from the database as resource objects.
// Users include a UserTrait because they are the 'shape' of a standard user.
func (u *userBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var rv []*v2.Resource
	users, err := u.client.GetUsers(ctx)
	if err != nil {
		return nil, "", nil, err
	}

	for _, user := range users {
		userCopy := user
		ur, err := userResource(parentResourceID, userCopy)
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
