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
				"namespace": node.Namespace,
			}),
		},
	)
}

// List returns all the nodes from the database as resource objects.
// Nodes include a NodeTrait because they are the 'shape' of a standard node.
func (n *nodeBuilder) List(ctx context.Context, _ *v2.ResourceId, opts rs.SyncOpAttrs) ([]*v2.Resource, *rs.SyncOpResults, error) {
	var rv []*v2.Resource
	resp, err := n.client.GetNodes(ctx, &pagination.Token{Token: opts.PageToken.Token})
	if err != nil {
		return nil, nil, fmt.Errorf("baton-teleport: failed to list nodes: %w", err)
	}

	for _, nodeWrapper := range resp.GetResources() {
		node := nodeWrapper.GetNode()
		rr, err := getNodeResource(&Node{
			Id:        node.GetRevision(),
			Name:      node.GetHostname(),
			Namespace: node.GetNamespace(),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("baton-teleport: failed to create node resource: %w", err)
		}
		rv = append(rv, rr)
	}

	return rv, &rs.SyncOpResults{NextPageToken: resp.NextKey}, nil
}

func (r *nodeBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ rs.SyncOpAttrs) ([]*v2.Entitlement, *rs.SyncOpResults, error) {
	return []*v2.Entitlement{
		ent.NewAssignmentEntitlement(
			resource,
			nodeMembership,
			ent.WithGrantableTo(userResourceType),
			ent.WithDisplayName(fmt.Sprintf("%s Node %s", resource.DisplayName, nodeMembership)),
			ent.WithDescription(fmt.Sprintf("Member of %s Teleport node", resource.DisplayName)),
		),
	}, nil, nil
}

// TODO: This should return grants based on who has access to the node resource
// ISSUE: TLDR: we need a way to associate nodes and roles
// ISSUE: this is more complicated than initially thought. we need to find what roles
// a user needs to access any given node, and then return the grants for those resources
// currently the GetAccessCapabilities should return these values, but is either erroring out
// or returning and empty list, we need to figure out a way to make that function run properly.
func (r *nodeBuilder) Grants(_ context.Context, _ *v2.Resource, _ rs.SyncOpAttrs) ([]*v2.Grant, *rs.SyncOpResults, error) {
	// nodes, err := r.client.ListResources(ctx, proto.ListResourcesRequest{
	// 	ResourceType: types.KindNode,
	// 	StartKey:     token.Token,
	// })

	// for _, n := range nodes.GetResources() {
	// 	accessCapabilitiesRequest, err := r.client.GetAccessCapabilities(ctx, types.AccessCapabilitiesRequest{
	// 		RequestableRoles: true,
	// 		ResourceIDs: []types.ResourceID{
	// 			{
	// 				ClusterName: n.GetNode().GetNamespace(),
	// 				Kind:        n.GetNode().GetKind(),
	// 				Name:        n.GetNode().GetName(),
	// 			},
	// 		},
	// 	})
	// 	if err != nil {
	// 		return nil, "", nil, err
	// 	}
	//
	//  NOTE: should return the resources applicable roles but is empty or errors out
	//	fmt.Println(fmt.Sprintf("accessCapabilitiesRequest.ApplicableRolesForResources: %+v", accessCapabilitiesRequest.ApplicableRolesForResources))
	// }
	//
	// for _, user := range users {
	// 	fmt.Println(fmt.Sprintf("roles: %+v", user.GetRoles())) // return's user's roles
	// 	fmt.Println(fmt.Sprintf("user.GetTraits(): %+v", user.GetTraits())) // returns user's resources
	// }

	return nil, nil, nil
}

// TODO: these should either grant/revoke access to a node, or we shouldn't implement them
// ISSUE: we need a way to associate nodes and roles.
func (r *nodeBuilder) Grant(_ context.Context, _ *v2.Resource, _ *v2.Entitlement) ([]*v2.Grant, annotations.Annotations, error) {
	// l := ctxzap.Extract(ctx)
	// userName := principal.Id.Resource
	// roleName := entitlement.Resource.Id.Resource
	//
	// if principal.Id.ResourceType != userResourceType.Id {
	// 	l.Warn(
	// 		"baton-segment: only users can be granted role membership",
	// 		zap.String("principal_type", principal.Id.ResourceType),
	// 		zap.String("principal_id", principal.Id.Resource),
	// 	)
	// 	return nil, fmt.Errorf("baton-segment: only users can be granted group membership")
	// }
	//
	// // TODO: check if node can be accessed with given entitlement
	//
	// //
	//
	// // Create an MFA required role for "prod" nodes.
	// prodRole, err := types.NewRole(roleName, types.RoleSpecV6{
	// 	Options: types.RoleOptions{
	// 		RequireMFAType: types.RequireMFAType_SESSION,
	// 	},
	// 	Allow: types.RoleConditions{
	// 		Logins:     []string{userName},
	// 		NodeLabels: types.Labels{},
	// 	},
	// })
	// if err != nil {
	// 	return nil, err
	// }
	//
	// user, err := r.client.GetUser(ctx, userName, false)
	// if err != nil {
	// 	return nil, err
	// }
	//
	// user.SetLogins(append(user.GetLogins(), userName))
	// user.AddRole(prodRole.GetName())
	// updatedUser, err := r.client.UpdateUser(ctx, user.(*types.UserV2))
	// if err != nil {
	// 	return nil, fmt.Errorf("teleport-connector: failed to add role: %s", err.Error())
	// }
	//
	// l.Warn("Role Membership has been created.",
	// 	zap.String("Name", updatedUser.GetName()),
	// 	zap.String("Namespace", updatedUser.GetMetadata().Namespace),
	// 	zap.Time("CreatedAt", updatedUser.GetCreatedBy().Time),
	// 	)
	//
	return nil, nil, nil
}

// TODO:
// ISSUE: we need a way to associate nodes and roles.
func (r *nodeBuilder) Revoke(_ context.Context, _ *v2.Grant) (annotations.Annotations, error) {
	return nil, nil
}

func newNodeBuilder(c *client.TeleportClient) *nodeBuilder {
	return &nodeBuilder{
		resourceType: nodeResourceType,
		client:       c,
	}
}
