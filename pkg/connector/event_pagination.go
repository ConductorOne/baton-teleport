package connector

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/conductorone/baton-sdk/pkg/pagination"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// eventPageCursor is the shared pagination state for all event feeds.
// It is JSON-marshalled and base64-encoded to produce the opaque cursor string
// stored in pagination.StreamState.Cursor.
type eventPageCursor struct {
	// StartAt is the lower bound of the time window for the next SearchEvents
	// call. After a full page cycle completes it is advanced to LatestEventSeen
	// so subsequent syncs only fetch new events.
	StartAt string `json:"start_at,omitempty"`

	// LatestEventSeen tracks the most recent event timestamp observed during
	// the current page cycle. Reset to "" when the cycle completes and StartAt
	// is advanced.
	LatestEventSeen string `json:"latest_event_seen,omitempty"`

	// LastKey is Teleport's opaque continuation key returned by SearchEvents.
	// It is passed back on the next call to resume mid-page.
	LastKey string `json:"last_key,omitempty"`
}

// unmarshalEventPageCursor deserialises the cursor from pToken, or initialises
// a fresh cursor whose StartAt is set to defaultStart (or 24 h ago when nil).
func unmarshalEventPageCursor(pToken *pagination.StreamToken, defaultStart *timestamppb.Timestamp) (*eventPageCursor, error) {
	c := &eventPageCursor{}
	if pToken != nil && pToken.Cursor != "" {
		data, err := base64.StdEncoding.DecodeString(pToken.Cursor)
		if err != nil {
			return nil, fmt.Errorf("baton-teleport: failed to decode page cursor: %w", err)
		}
		if err := json.Unmarshal(data, c); err != nil {
			return nil, fmt.Errorf("baton-teleport: failed to unmarshal page cursor: %w", err)
		}
	}

	if c.StartAt == "" {
		var start time.Time
		if defaultStart != nil {
			start = defaultStart.AsTime()
		} else {
			start = time.Now().UTC().Add(-24 * time.Hour)
		}
		c.StartAt = start.UTC().Format(time.RFC3339Nano)
	}

	if c.LatestEventSeen == "" {
		c.LatestEventSeen = c.StartAt
	}

	return c, nil
}

// marshal serialises the cursor to a base64-encoded JSON string ready to be
// stored in pagination.StreamState.Cursor.
func (c *eventPageCursor) marshal() (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("baton-teleport: failed to marshal page cursor: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// updateLatestEvent advances LatestEventSeen if t is strictly after the
// currently recorded timestamp. Zero times are ignored.
func (c *eventPageCursor) updateLatestEvent(t time.Time) {
	if t.IsZero() {
		return
	}
	latest, err := time.Parse(time.RFC3339Nano, c.LatestEventSeen)
	if err != nil || t.After(latest) {
		c.LatestEventSeen = t.UTC().Format(time.RFC3339Nano)
	}
}

// prepareNextSync advances StartAt to LatestEventSeen once a full page cycle
// has completed, so the next call only fetches events that occurred after the
// last one we processed.
func (c *eventPageCursor) prepareNextSync() {
	c.StartAt = c.LatestEventSeen
	c.LatestEventSeen = ""
	c.LastKey = ""
}
