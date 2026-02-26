package connector

import (
	"context"
	"fmt"
	"io"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"

	"github.com/conductorone/baton-teleport/pkg/client"
)

type Connector struct {
	client *client.TeleportClient
}

// ResourceSyncers returns a ResourceSyncer for each resource type that should be synced from the upstream service.
func (d *Connector) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncer {
	return []connectorbuilder.ResourceSyncer{
		newUserBuilder(d.client),
		newRoleBuilder(d.client),
		newNodeBuilder(d.client),
		newAppBuilder(d.client),
		newDatabaseBuilder(d.client),
	}
}

// Asset takes an input AssetRef and attempts to fetch it using the connector's authenticated http client
// It streams a response, always starting with a metadata object, following by chunked payloads for the asset.
func (d *Connector) Asset(ctx context.Context, asset *v2.AssetRef) (string, io.ReadCloser, error) {
	return "", nil, nil
}

func (d *Connector) Metadata(_ context.Context) (*v2.ConnectorMetadata, error) {
	return &v2.ConnectorMetadata{
		DisplayName: "Teleport Connector",
		Description: "Connector to sync and provision users into Teleport.",
		AccountCreationSchema: &v2.ConnectorAccountCreationSchema{
			FieldMap: map[string]*v2.ConnectorAccountCreationSchema_Field{
				"name": {
					DisplayName: "Username",
					Required:    true,
					Description: "The unique username of the user in Teleport.",
					Field: &v2.ConnectorAccountCreationSchema_Field_StringField{
						StringField: &v2.ConnectorAccountCreationSchema_StringField{},
					},
					Placeholder: "name",
					Order:       1,
				},
				"role": {
					DisplayName: "Role",
					Required:    false,
					Description: "The role to assign to the user. Defaults to 'access' if not provided.",
					Field: &v2.ConnectorAccountCreationSchema_Field_StringField{
						StringField: &v2.ConnectorAccountCreationSchema_StringField{},
					},
					Placeholder: "access",
					Order:       2,
				},
			},
		},
	}, nil
}

func (d *Connector) Validate(ctx context.Context) (annotations.Annotations, error) {
	_, err := d.client.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("baton-teleport: failed to validate connection: %w", err)
	}
	return nil, nil
}

func (d *Connector) Close() error {
	if d.client != nil {
		return d.client.Close()
	}
	return nil
}

func (d *Connector) EventFeeds(_ context.Context) []connectorbuilder.EventFeed {
	return []connectorbuilder.EventFeed{
		newUsageEventFeed(d.client),
		newAuditEventFeed(d.client),
	}
}

// New returns a new instance of the connector.
func New(
	ctx context.Context,
	proxyAddress string,
	keyFilePath string,
	key string,
) (*Connector, error) {
	tc, err := client.New(ctx, proxyAddress, keyFilePath, key)
	if err != nil {
		return nil, err
	}

	return &Connector{
		client: tc,
	}, nil
}
