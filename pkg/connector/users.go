package connector

import (
	"context"
	"fmt"
	"time"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-teleport/pkg/client"
	"github.com/gravitational/teleport/api/client/proto"
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

	if user.IsBot() {
		accountType = v2.UserTrait_ACCOUNT_TYPE_SERVICE
	}

	if types.IsSystemResource(user) {
		accountType = v2.UserTrait_ACCOUNT_TYPE_SYSTEM
	}

	name := user.GetName()

	firstName, lastName := splitDashSeparatedName(name)
	profile := map[string]interface{}{
		"name":       name,
		"user_id":    user.GetMetadata().Revision,
		"first_name": firstName,
		"last_name":  lastName,
	}

	// Teleport does not store an email natively for users.
	if accountType == v2.UserTrait_ACCOUNT_TYPE_HUMAN {
		profile["email"] = name
	}

	switch user.GetStatus().IsLocked {
	case true:
		status = v2.UserTrait_Status_STATUS_DISABLED
	case false:
		status = v2.UserTrait_Status_STATUS_ENABLED
	default:
		status = v2.UserTrait_Status_STATUS_UNSPECIFIED
	}

	opts := []resource.UserTraitOption{
		resource.WithUserProfile(profile),
		resource.WithUserLogin(name),
		resource.WithStatus(status),
		resource.WithAccountType(accountType),
	}

	if accountType == v2.UserTrait_ACCOUNT_TYPE_HUMAN {
		opts = append(opts, resource.WithEmail(name, true))
	}
	return resource.NewUserResource(
		name,
		userResourceType,
		name,
		opts,
		resource.WithParentResourceID(pId),
	)
}

func (u *userBuilder) CreateAccountCapabilityDetails(_ context.Context) (*v2.CredentialDetailsAccountProvisioning, annotations.Annotations, error) {
	return &v2.CredentialDetailsAccountProvisioning{
		SupportedCredentialOptions: []v2.CapabilityDetailCredentialOption{
			v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_NO_PASSWORD,
		},
		PreferredCredentialOption: v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_NO_PASSWORD,
	}, nil, nil
}

func (u *userBuilder) CreateAccount(
	ctx context.Context,
	accountInfo *v2.AccountInfo,
	_ *v2.CredentialOptions,
) (connectorbuilder.CreateAccountResponse, []*v2.PlaintextData, annotations.Annotations, error) {
	newUser, err := createNewUserInfo(accountInfo)
	if err != nil {
		return nil, nil, nil, err
	}

	_, err = u.client.CreateUser(ctx, newUser)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create user: %w", err)
	}

	token, err := u.client.CreateResetPasswordToken(ctx, &proto.CreateResetPasswordTokenRequest{
		Name: newUser.GetName(),
		TTL:  proto.Duration(24 * time.Hour),
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create reset password token: %w", err)
	}

	userRes, err := userResource(nil, newUser)
	if err != nil {
		return nil, nil, nil, err
	}

	passwordConfigurationLink := &v2.PlaintextData{
		Name:  "password_configuration_link",
		Bytes: []byte(token.GetURL()),
	}

	caResponse := &v2.CreateAccountResponse_SuccessResult{
		Resource: userRes,
	}

	return caResponse, []*v2.PlaintextData{passwordConfigurationLink}, nil, nil
}

func createNewUserInfo(accountInfo *v2.AccountInfo) (*types.UserV2, error) {
	p := accountInfo.GetProfile().AsMap()

	username, ok := p["name"].(string)
	if !ok || username == "" {
		return nil, fmt.Errorf("missing required field: name")
	}

	// NOTE: In Teleport, every user must be assigned at least one role upon creation.
	role, _ := p["role"].(string)
	if role == "" {
		role = "access"
	}

	// Teleport usernames must be formed by joining the user's first and last name with a dash (`-`).
	// This is a common convention required when provisioning new users.
	name := cleanResourceName(username)

	return &types.UserV2{
		Kind:    types.KindUser,
		Version: types.V2,
		Metadata: types.Metadata{
			Name: name,
		},
		Spec: types.UserSpecV2{
			Roles: []string{role},
			Traits: map[string][]string{
				"logins": {username},
			},
		},
	}, nil
}

func (u *userBuilder) Delete(ctx context.Context, resourceID *v2.ResourceId) (annotations.Annotations, error) {
	username := resourceID.GetResource()
	if username == "" {
		return nil, fmt.Errorf("missing resource name")
	}

	user, err := u.client.GetUser(ctx, username, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if user.GetUserType() != types.UserTypeLocal {
		return nil, fmt.Errorf("cannot delete federated (non-local) user: %s", username)
	}

	err = u.client.DeleteUser(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to delete user: %w", err)
	}

	return nil, nil
}

// List returns all the users from the database as resource objects.
// Users include a UserTrait because they are the 'shape' of a standard user.
func (u *userBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var rv []*v2.Resource
	users, err := u.client.GetUsers(ctx, false)
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
