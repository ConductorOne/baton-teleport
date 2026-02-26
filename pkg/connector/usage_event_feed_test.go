package connector

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/gravitational/teleport/api/types/events"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// --- unmarshalEventPageCursor ---

func TestUnmarshalEventPageCursor_NilToken(t *testing.T) {
	defaultStart := timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	c, err := unmarshalEventPageCursor(nil, defaultStart)
	require.NoError(t, err)
	require.Equal(t, "2024-01-01T00:00:00Z", c.StartAt)
	require.Equal(t, c.StartAt, c.LatestEventSeen)
	require.Empty(t, c.LastKey)
}

func TestUnmarshalEventPageCursor_EmptyCursor(t *testing.T) {
	defaultStart := timestamppb.New(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	c, err := unmarshalEventPageCursor(&pagination.StreamToken{Cursor: ""}, defaultStart)
	require.NoError(t, err)
	require.Equal(t, "2024-06-15T12:00:00Z", c.StartAt)
	require.Equal(t, c.StartAt, c.LatestEventSeen)
}

func TestUnmarshalEventPageCursor_NilEarliestEvent(t *testing.T) {
	before := time.Now().UTC().Add(-25 * time.Hour)
	c, err := unmarshalEventPageCursor(nil, nil)
	require.NoError(t, err)
	parsed, err := time.Parse(time.RFC3339Nano, c.StartAt)
	require.NoError(t, err)
	require.True(t, parsed.After(before), "StartAt should be after 25h ago")
	require.True(t, parsed.Before(time.Now()), "StartAt should be before now")
}

func TestUnmarshalEventPageCursor_ValidCursor(t *testing.T) {
	original := &eventPageCursor{
		StartAt:         "2024-03-01T00:00:00Z",
		LatestEventSeen: "2024-03-01T08:00:00Z",
		LastKey:         "some-teleport-key",
	}
	data, err := json.Marshal(original)
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(data)

	c, err := unmarshalEventPageCursor(&pagination.StreamToken{Cursor: encoded}, nil)
	require.NoError(t, err)
	require.Equal(t, original.StartAt, c.StartAt)
	require.Equal(t, original.LatestEventSeen, c.LatestEventSeen)
	require.Equal(t, original.LastKey, c.LastKey)
}

func TestUnmarshalEventPageCursor_InvalidBase64(t *testing.T) {
	_, err := unmarshalEventPageCursor(&pagination.StreamToken{Cursor: "not-valid-base64!!!"}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "baton-teleport: failed to decode page cursor")
}

func TestUnmarshalEventPageCursor_InvalidJSON(t *testing.T) {
	bad := base64.StdEncoding.EncodeToString([]byte("{not json}"))
	_, err := unmarshalEventPageCursor(&pagination.StreamToken{Cursor: bad}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "baton-teleport: failed to unmarshal page cursor")
}

// --- marshal / roundtrip ---

func TestEventPageCursor_MarshalRoundtrip(t *testing.T) {
	c := &eventPageCursor{
		StartAt:         "2024-05-10T10:00:00Z",
		LatestEventSeen: "2024-05-10T11:30:00Z",
		LastKey:         "abc123",
	}

	encoded, err := c.marshal()
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded, err := unmarshalEventPageCursor(&pagination.StreamToken{Cursor: encoded}, nil)
	require.NoError(t, err)
	require.Equal(t, c.StartAt, decoded.StartAt)
	require.Equal(t, c.LatestEventSeen, decoded.LatestEventSeen)
	require.Equal(t, c.LastKey, decoded.LastKey)
}

// --- updateLatestEvent ---

func TestUpdateLatestEvent_UpdatesWhenNewer(t *testing.T) {
	c := &eventPageCursor{LatestEventSeen: "2024-01-01T00:00:00Z"}
	newer := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	c.updateLatestEvent(newer)
	require.Equal(t, "2024-01-02T00:00:00Z", c.LatestEventSeen)
}

func TestUpdateLatestEvent_NoChangeWhenOlder(t *testing.T) {
	c := &eventPageCursor{LatestEventSeen: "2024-06-01T00:00:00Z"}
	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	c.updateLatestEvent(older)
	require.Equal(t, "2024-06-01T00:00:00Z", c.LatestEventSeen)
}

func TestUpdateLatestEvent_ZeroTimeIsNoop(t *testing.T) {
	c := &eventPageCursor{LatestEventSeen: "2024-06-01T00:00:00Z"}
	c.updateLatestEvent(time.Time{})
	require.Equal(t, "2024-06-01T00:00:00Z", c.LatestEventSeen)
}

func TestUpdateLatestEvent_BadLatestEventSeenFallsBack(t *testing.T) {
	c := &eventPageCursor{LatestEventSeen: "not-a-date"}
	t1 := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	c.updateLatestEvent(t1)
	require.Equal(t, "2024-03-01T00:00:00Z", c.LatestEventSeen)
}

// --- prepareNextSync ---

func TestPrepareNextSync_AdvancesStartAt(t *testing.T) {
	c := &eventPageCursor{
		StartAt:         "2024-01-01T00:00:00Z",
		LatestEventSeen: "2024-01-01T05:00:00Z",
		LastKey:         "page-key",
	}
	c.prepareNextSync()
	require.Equal(t, "2024-01-01T05:00:00Z", c.StartAt)
	require.Empty(t, c.LatestEventSeen)
	require.Empty(t, c.LastKey)
}

func TestCursorKeepsLastKeyWhenMorePages(t *testing.T) {
	c := &eventPageCursor{
		StartAt: "2024-01-01T00:00:00Z",
	}
	c.LastKey = "page-2-key"
	require.Equal(t, "2024-01-01T00:00:00Z", c.StartAt)
	require.Equal(t, "page-2-key", c.LastKey)
}

// --- convertToUsageEvent ---

func TestConvertToUsageEvent_Success(t *testing.T) {
	eventTime := time.Date(2024, 4, 10, 9, 0, 0, 0, time.UTC)
	login := &events.UserLogin{
		Metadata: events.Metadata{
			ClusterName: "my-cluster",
			ID:          "event-abc",
			Time:        eventTime,
		},
		UserMetadata: events.UserMetadata{
			User: "alice",
		},
		Status: events.Status{
			Success: true,
		},
	}

	evt, ts := convertToUsageEvent(login)
	require.NotNil(t, evt)
	require.Equal(t, "event-abc", evt.Id)
	require.Equal(t, eventTime.Unix(), ts.Unix())

	usageEvt := evt.GetUsageEvent()
	require.NotNil(t, usageEvt)

	require.NotNil(t, usageEvt.TargetResource)
	require.Equal(t, teleportClusterResource, usageEvt.TargetResource.Id.ResourceType)
	require.Equal(t, "my-cluster", usageEvt.TargetResource.Id.Resource)
	require.Equal(t, "my-cluster", usageEvt.TargetResource.DisplayName)

	require.NotNil(t, usageEvt.ActorResource)
	require.Equal(t, userResourceType.Id, usageEvt.ActorResource.Id.ResourceType)
	require.Equal(t, "alice", usageEvt.ActorResource.Id.Resource)
	require.Equal(t, "alice", usageEvt.ActorResource.DisplayName)
}

func TestConvertToUsageEvent_FailedLogin(t *testing.T) {
	login := &events.UserLogin{
		Metadata:     events.Metadata{ID: "fail-event"},
		UserMetadata: events.UserMetadata{User: "bob"},
		Status:       events.Status{Success: false},
	}
	evt, ts := convertToUsageEvent(login)
	require.Nil(t, evt)
	require.True(t, ts.IsZero())
}

func TestConvertToUsageEvent_WrongEventType(t *testing.T) {
	other := &events.SessionStart{}
	evt, ts := convertToUsageEvent(other)
	require.Nil(t, evt)
	require.True(t, ts.IsZero())
}

func TestConvertToUsageEvent_EmptyClusterNameFallback(t *testing.T) {
	login := &events.UserLogin{
		Metadata:     events.Metadata{ID: "e1", ClusterName: ""},
		UserMetadata: events.UserMetadata{User: "carol"},
		Status:       events.Status{Success: true},
	}
	evt, _ := convertToUsageEvent(login)
	require.NotNil(t, evt)
	require.Equal(t, "teleport", evt.GetUsageEvent().TargetResource.Id.Resource)
}
