package connector

import (
	"context"
	"fmt"
	"time"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	apidefaults "github.com/gravitational/teleport/api/defaults"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/conductorone/baton-teleport/pkg/client"
)

const (
	usageEventFeedID        = "teleport_usage_events"
	eventsPageSize          = 100
	userLoginEventType      = "user.login"
	teleportClusterResource = "teleport_cluster"
)

type usageEventFeed struct {
	client *client.TeleportClient
}

func (e *usageEventFeed) EventFeedMetadata(_ context.Context) *v2.EventFeedMetadata {
	return &v2.EventFeedMetadata{
		Id: usageEventFeedID,
		SupportedEventTypes: []v2.EventType{
			v2.EventType_EVENT_TYPE_USAGE,
		},
	}
}

func (e *usageEventFeed) ListEvents(
	ctx context.Context,
	earliestEvent *timestamppb.Timestamp,
	pToken *pagination.StreamToken,
) ([]*v2.Event, *pagination.StreamState, annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	cursor, err := unmarshalEventPageCursor(pToken, earliestEvent)
	if err != nil {
		return nil, nil, nil, err
	}

	from, err := time.Parse(time.RFC3339Nano, cursor.StartAt)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("baton-teleport: failed to parse start_at: %w", err)
	}
	to := time.Now().UTC()

	auditEvents, lastKey, err := e.client.SearchEvents(
		ctx,
		from,
		to,
		apidefaults.Namespace,
		[]string{userLoginEventType},
		eventsPageSize,
		types.EventOrderAscending,
		cursor.LastKey,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("baton-teleport: failed to search usage events: %w", err)
	}

	l.Debug("fetched usage events",
		zap.Int("count", len(auditEvents)),
		zap.String("last_key", lastKey),
	)

	var result []*v2.Event
	for _, auditEvent := range auditEvents {
		evt, eventTime := convertToUsageEvent(auditEvent)
		cursor.updateLatestEvent(eventTime)
		if evt != nil {
			result = append(result, evt)
		}
	}

	if lastKey == "" {
		cursor.prepareNextSync()
	} else {
		cursor.LastKey = lastKey
	}

	marshalledToken, err := cursor.marshal()
	if err != nil {
		return nil, nil, nil, err
	}

	return result, &pagination.StreamState{
		Cursor:  marshalledToken,
		HasMore: lastKey != "",
	}, nil, nil
}

func convertToUsageEvent(auditEvent events.AuditEvent) (*v2.Event, time.Time) {
	userLogin, ok := auditEvent.(*events.UserLogin)
	if !ok {
		return nil, time.Time{}
	}

	eventTime := userLogin.GetTime()

	if !userLogin.Success {
		return nil, eventTime
	}

	clusterName := userLogin.GetClusterName()
	if clusterName == "" {
		clusterName = "teleport"
	}

	return &v2.Event{
		Id:         userLogin.GetID(),
		OccurredAt: timestamppb.New(eventTime),
		Event: &v2.Event_UsageEvent{
			UsageEvent: &v2.UsageEvent{
				TargetResource: &v2.Resource{
					Id: &v2.ResourceId{
						ResourceType: teleportClusterResource,
						Resource:     clusterName,
					},
					DisplayName: clusterName,
				},
				ActorResource: &v2.Resource{
					Id: &v2.ResourceId{
						ResourceType: userResourceType.Id,
						Resource:     userLogin.User,
					},
					DisplayName: userLogin.User,
				},
			},
		},
	}, eventTime
}

func newUsageEventFeed(c *client.TeleportClient) *usageEventFeed {
	return &usageEventFeed{client: c}
}
