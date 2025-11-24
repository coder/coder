package usagetypes_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/usage/usagetypes"
)

func TestTallymanV1UsageEvent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		event        usagetypes.TallymanV1IngestEvent
		errorMessage string
	}{
		{
			name: "OK",
			event: usagetypes.TallymanV1IngestEvent{
				ID:        "123",
				EventType: usagetypes.UsageEventTypeDCManagedAgentsV1,
				// EventData is not validated.
				EventData: json.RawMessage{},
				CreatedAt: time.Now(),
			},
			errorMessage: "",
		},
		{
			name: "NoID",
			event: usagetypes.TallymanV1IngestEvent{
				EventType: usagetypes.UsageEventTypeDCManagedAgentsV1,
				EventData: json.RawMessage{},
				CreatedAt: time.Now(),
			},
			errorMessage: "id is required",
		},
		{
			name: "NoEventType",
			event: usagetypes.TallymanV1IngestEvent{
				ID:        "123",
				EventType: usagetypes.UsageEventType(""),
				EventData: json.RawMessage{},
				CreatedAt: time.Now(),
			},
			errorMessage: `event_type "" is invalid`,
		},
		{
			name: "UnknownEventType",
			event: usagetypes.TallymanV1IngestEvent{
				ID:        "123",
				EventType: usagetypes.UsageEventType("unknown"),
				EventData: json.RawMessage{},
				CreatedAt: time.Now(),
			},
			errorMessage: `event_type "unknown" is invalid`,
		},
		{
			name: "NoCreatedAt",
			event: usagetypes.TallymanV1IngestEvent{
				ID:        "123",
				EventType: usagetypes.UsageEventTypeDCManagedAgentsV1,
				EventData: json.RawMessage{},
				CreatedAt: time.Time{},
			},
			errorMessage: "created_at cannot be zero",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.event.Valid()
			if tc.errorMessage == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.errorMessage)
			}
		})
	}
}
