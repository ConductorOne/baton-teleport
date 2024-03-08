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

func (t *TeleportClient) GetUsers(ctx context.Context) ([]types.User, error) {
	users, err := t.client.GetUsers(ctx, false)
	if err != nil {
		return nil, err
	}

	return users, nil
}
