package connector

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-teleport/pkg/client"
	teleport "github.com/gravitational/teleport/api/client"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

var (
	ctxTest       = context.Background()
	proxyAddrTest = "conductorone-c1.teleport.sh:443"
	filePathTest  = "../../auth.pem"
)

func Test_roleBuilderList(t *testing.T) {
	if !checkFileExists(filePathTest) {
		t.Skip()
	}

	cliTest, err := getTestingClient(ctxTest, proxyAddrTest, filePathTest)
	require.Nil(t, err)

	r := &roleBuilder{
		resourceType: &v2.ResourceType{},
		client:       cliTest,
	}
	_, _, _, err = r.List(ctxTest, &v2.ResourceId{}, &pagination.Token{})
	require.Nil(t, err)
}

func getTestingClient(ctx context.Context, proxyAddr, filePath string) (*client.TeleportClient, error) {
	tc, err := NewTestingClient(ctx, proxyAddr, filePath)
	if err != nil {
		return nil, err
	}

	return tc, nil
}

func NewTestingClient(ctx context.Context, proxyAddr, file string) (*client.TeleportClient, error) {
	const initTimeout = time.Duration(10) * time.Second
	tc := &client.TeleportClient{
		ProxyAddr: proxyAddr,
	}
	ctx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()

	creds := teleport.LoadIdentityFile(file)
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

	return tc.SetClient(ctx, client), nil
}

func checkFileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !errors.Is(err, os.ErrNotExist)
}
