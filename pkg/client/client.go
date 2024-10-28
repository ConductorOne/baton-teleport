package client

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/conductorone/baton-sdk/pkg/pagination"
	teleport "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
)

type TeleportClient struct {
	*teleport.Client
	ProxyAddress string
}

var ErrNoKeyProvided = errors.New("no key provided")

const initTimeout = time.Duration(10) * time.Second

func New(ctx context.Context, proxyAddress, keyFile, key string) (*TeleportClient, error) {
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

	tc.Client = client
	return tc, nil
}

func hasPort(address string) bool {
	// remove https and http if it has it
	address = strings.TrimPrefix(address, "https://")
	address = strings.TrimPrefix(address, "http://")
	return len(strings.Split(address, ":")) == 2
}

func (t *TeleportClient) GetNodes(ctx context.Context, token *pagination.Token) (*proto.ListResourcesResponse, error) {
	return t.Client.GetResources(ctx, &proto.ListResourcesRequest{
		ResourceType: types.KindNode,
		StartKey:     token.Token,
	})
}
