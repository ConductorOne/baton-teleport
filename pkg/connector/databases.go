package connector

import (
	"context"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/gravitational/teleport/api/types"

	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-teleport/pkg/client"
)

const dbMembership = "member"

type dbBuilder struct {
	resourceType *v2.ResourceType
	client       *client.TeleportClient
}

func (d *dbBuilder) ResourceType(_ context.Context) *v2.ResourceType {
	return d.resourceType
}

// Create a new connector resource for a Teleport node.
func getDatabaseResource(db types.Database) (*v2.Resource, error) {
	dbId := db.GetRevision()
	return rs.NewRoleResource(
		db.GetName(),
		dbResourceType,
		dbId,
		[]rs.RoleTraitOption{
			rs.WithRoleProfile(
				map[string]interface{}{
					"db_id":   dbId,
					"db_name": db.GetName(),
				},
			),
		},
	)
}

// List returns all the databases from the database as resource objects.
// Databases include a NodeTrait because they are the 'shape' of a standard db.
func (d *dbBuilder) List(ctx context.Context, parentId *v2.ResourceId, token *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var rv []*v2.Resource
	databases, err := d.client.GetDatabases(ctx)
	if err != nil {
		return nil, "", nil, err
	}

	for _, db := range databases {
		dbCopy := db
		rr, err := getDatabaseResource(dbCopy)
		if err != nil {
			return nil, "", nil, err
		}
		rv = append(rv, rr)
	}

	return rv, "", nil, nil
}

func (d *dbBuilder) Entitlements(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return []*v2.Entitlement{
		ent.NewAssignmentEntitlement(
			resource,
			dbMembership,
			ent.WithGrantableTo(userResourceType),
			ent.WithDisplayName(fmt.Sprintf("%s Database %s", resource.DisplayName, dbMembership)),
			ent.WithDescription(fmt.Sprintf("Member of %s Teleport db", resource.DisplayName)),
		),
	}, "", nil, nil
}

func (d *dbBuilder) Grants(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func (a *dbBuilder) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) (annotations.Annotations, error) {
	return nil, nil
}

func (d *dbBuilder) Revoke(ctx context.Context, grant *v2.Grant) (annotations.Annotations, error) {
	return nil, nil
}

func newDatabaseBuilder(c *client.TeleportClient) *dbBuilder {
	return &dbBuilder{
		resourceType: dbResourceType,
		client:       c,
	}
}
