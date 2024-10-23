package client

import (
	"context"
	"errors"
	"strings"
	"time"

	teleport "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
)

type TeleportClient struct {
	client       *teleport.Client
	ProxyAddress string
}

var ErrNoKeyProvided = errors.New("no key provided")

const initTimeout = time.Duration(10) * time.Second

func New(ctx context.Context, proxyAddress, keyFile, key string) (*TeleportClient, error) {
	// DONE: Dial opts are deprecated. We also need to add a default port in proxyAddress if one doesn't exist (to avoid an info message)
	if !hasPort(proxyAddress) {
		proxyAddress += ":443"
	}

	tc := &TeleportClient{
		ProxyAddress: proxyAddress,
	}
	ctx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()

	var creds teleport.Credentials
	switch {
	case keyFile != "":
		creds = teleport.LoadIdentityFile(keyFile)
	case key != "":
		creds = teleport.LoadIdentityFileFromString(key)
	default:
		return nil, ErrNoKeyProvided
	}

	client, err := teleport.New(ctx, teleport.Config{
		Addrs:       []string{proxyAddress},
		Credentials: []teleport.Credentials{creds},
	})
	if err != nil {
		return nil, err
	}

	tc.SetClient(ctx, client)
	return tc, nil
}

func (t *TeleportClient) SetClient(ctx context.Context, c *teleport.Client) {
	t.client = c
}

// TODO: why wrap every client method? We should probably just make the client public

// GetUsers fetch users list.
func (t *TeleportClient) GetUsers(ctx context.Context) ([]types.User, error) {
	return t.client.GetUsers(ctx, false)
}

// GetRoles fetch roles list.
func (t *TeleportClient) GetRoles(ctx context.Context) ([]types.Role, error) {
	return t.client.GetRoles(ctx)
func hasPort(address string) bool {
	// remove https and http if it has it
	address = strings.TrimPrefix(address, "https://")
	address = strings.TrimPrefix(address, "http://")
	return len(strings.Split(address, ":")) == 2
}

// GetUser gets a user.
func (t *TeleportClient) GetUser(ctx context.Context, username string) (types.User, error) {
	return t.client.GetUser(ctx, username, false)
}

// UpdateUserRole updates a user.
func (t *TeleportClient) UpdateUserRole(ctx context.Context, user types.User) (types.User, error) {
	return t.client.UpdateUser(ctx, user.(*types.UserV2))
}

func (t *TeleportClient) GetNodes(ctx context.Context) (*proto.ListResourcesResponse, error) {
	return t.client.GetResources(ctx, &proto.ListResourcesRequest{
		ResourceType: types.KindNode,
	})
}

func (t *TeleportClient) GetApps(ctx context.Context) ([]types.Application, error) {
	return t.client.GetApps(ctx)
}

func (t *TeleportClient) GetDatabases(ctx context.Context) ([]types.Database, error) {
	return t.client.GetDatabases(ctx)
}
