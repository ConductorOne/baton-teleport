package connector

import (
	"context"
	"fmt"
	"time"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	teleport "github.com/gravitational/teleport/api/client"
	"google.golang.org/grpc"
)

type userBuilder struct {
	resourceType   *v2.ResourceType
	teleportClient *teleport.Client
}

const (
	// Assign proxyAddr to the host and port of your Teleport Proxy Service instance
	proxyAddr      string = "d3v-conductorone.teleport.sh:443"
	initTimeout           = time.Duration(10) * time.Second
	updateInterval        = time.Duration(5) * time.Second
	tokenTTL              = time.Duration(5) * time.Minute
	networkName    string = "bridge"
	managementPort string = "15672"
	tokenLenBytes         = 16
)

func (o *userBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return userResourceType
}

// List returns all the users from the database as resource objects.
// Users include a UserTrait because they are the 'shape' of a standard user.
func (o *userBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	ctx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	creds := teleport.LoadIdentityFile("auth.pem")

	t, err := teleport.New(ctx, teleport.Config{
		Addrs:       []string{proxyAddr},
		Credentials: []teleport.Credentials{creds},
		DialOpts: []grpc.DialOption{
			grpc.WithReturnConnectionError(),
		},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("Connected to Teleport")

	users, err := t.GetUsers(ctx, false)
	if err != nil {
		panic(err)
	}
	fmt.Println("Teleport Users")
	fmt.Println(users)
	return nil, "", nil, nil
}

// Entitlements always returns an empty slice for users.
func (o *userBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

// Grants always returns an empty slice for users since they don't have any entitlements.
func (o *userBuilder) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func newUserBuilder() *userBuilder {
	return &userBuilder{}
}
