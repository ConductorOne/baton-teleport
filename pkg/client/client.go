package client

import (
	"context"
	"time"

	teleport "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
	"google.golang.org/grpc"
)

type TeleportClient struct {
	client    *teleport.Client
	ProxyAddr string
}

const initTimeout = time.Duration(10) * time.Second

func New(ctx context.Context, proxyAddr string) (*TeleportClient, error) {
	tc := &TeleportClient{
		ProxyAddr: proxyAddr,
	}
	ctx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	creds := teleport.LoadIdentityFile("auth.pem")

	client, err := teleport.New(ctx, teleport.Config{
		Addrs:       []string{proxyAddr},
		Credentials: []teleport.Credentials{creds},
		DialOpts: []grpc.DialOption{
			grpc.WithReturnConnectionError(),
		},
	})
	if err != nil {
		return nil, err
	}

	tc.client = client
	return tc, nil
}

// GetUsers fetch users list.
func (t *TeleportClient) GetUsers(ctx context.Context) ([]types.User, error) {
	users, err := t.client.GetUsers(ctx, false)
	if err != nil {
		return nil, err
	}

	return users, nil
}

// GetRoles fetch roles list.
func (t *TeleportClient) GetRoles(ctx context.Context) ([]types.Role, error) {
	roles, err := t.client.GetRoles(ctx)
	if err != nil {
		return nil, err
	}

	return roles, nil
}

// GetUser gets an user.
func (t *TeleportClient) GetUser(ctx context.Context, username string) (types.User, error) {
	user, err := t.client.GetUser(ctx, username, false)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// UpdateUserRole updates an user.
func (t *TeleportClient) UpdateUserRole(ctx context.Context, user types.User) (types.User, error) {
	updatedUser, err := t.client.UpdateUser(ctx, user.(*types.UserV2))
	if err != nil {
		return nil, err
	}

	return updatedUser, nil
}
