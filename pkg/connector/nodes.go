package connector

import (
	"context"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"

	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/conductorone/baton-teleport/pkg/client"
)

const nodeMembership = "member"

type nodeBuilder struct {
	resourceType *v2.ResourceType
	client       *client.TeleportClient
}

type Node struct {
	Id        string
	Name      string
	Namespace string
}

var mapNodes = make(map[string]Node)

func (n *nodeBuilder) ResourceType(_ context.Context) *v2.ResourceType {
	return n.resourceType
}

// Create a new connector resource for a Teleport node.
func getNodeResource(node *Node) (*v2.Resource, error) {
	return rs.NewRoleResource(
		node.Name,
		nodeResourceType,
		node.Id,
		[]rs.RoleTraitOption{
			rs.WithRoleProfile(map[string]interface{}{
				"node_id":   node.Id,
				"node_name": node.Name,
			}),
		},
	)
}

// List returns all the nodes from the database as resource objects.
// Nodes include a NodeTrait because they are the 'shape' of a standard node.
func (n *nodeBuilder) List(ctx context.Context, parentId *v2.ResourceId, token *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var rv []*v2.Resource
	nodes, err := n.client.GetNodes(ctx)
	if err != nil {
		return nil, "", nil, err
	}

	for _, node := range nodes.GetResources() {
		id := node.GetNode().GetRevision()
		mapNodes[id] = Node{
			Id:        id,
			Name:      node.GetNode().GetName(),
			Namespace: node.GetNode().GetNamespace(),
		}
	}

	for _, node := range mapNodes {
		nodeCopy := node
		rr, err := getNodeResource(&nodeCopy)
		if err != nil {
			return nil, "", nil, err
		}
		rv = append(rv, rr)
	}

	return rv, "", nil, nil
}

func (r *nodeBuilder) Entitlements(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return []*v2.Entitlement{
		ent.NewAssignmentEntitlement(
			resource,
			nodeMembership,
			ent.WithGrantableTo(userResourceType),
			ent.WithDisplayName(fmt.Sprintf("%s Node %s", resource.DisplayName, nodeMembership)),
			ent.WithDescription(fmt.Sprintf("Member of %s Teleport node", resource.DisplayName)),
		),
	}, "", nil, nil
}

func (r *nodeBuilder) Grants(ctx context.Context, resource *v2.Resource, token *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func (r *nodeBuilder) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) (annotations.Annotations, error) {
	return nil, nil
}

func (r *nodeBuilder) Revoke(ctx context.Context, grant *v2.Grant) (annotations.Annotations, error) {
	return nil, nil
}

func newNodeBuilder(c *client.TeleportClient) *nodeBuilder {
	return &nodeBuilder{
		resourceType: nodeResourceType,
		client:       c,
	}
}
