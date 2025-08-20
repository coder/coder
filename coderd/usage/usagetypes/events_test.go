package usagetypes_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/usage/usagetypes"
)

func TestParseEvent(t *testing.T) {
	t.Parallel()

	t.Run("ExtraFields", func(t *testing.T) {
		t.Parallel()
		_, err := usagetypes.ParseEvent[usagetypes.DCManagedAgentsV1]([]byte(`{"count": 1, "extra": "field"}`))
		require.ErrorContains(t, err, "unmarshal usagetypes.DCManagedAgentsV1 event")
	})

	t.Run("ExtraData", func(t *testing.T) {
		t.Parallel()
		_, err := usagetypes.ParseEvent[usagetypes.DCManagedAgentsV1]([]byte(`{"count": 1}{"count": 2}`))
		require.ErrorContains(t, err, "extra data after usagetypes.DCManagedAgentsV1 event")
	})

	t.Run("DCManagedAgentsV1", func(t *testing.T) {
		t.Parallel()

		event, err := usagetypes.ParseEvent[usagetypes.DCManagedAgentsV1]([]byte(`{"count": 1}`))
		require.NoError(t, err)
		require.Equal(t, usagetypes.DCManagedAgentsV1{Count: 1}, event)
		require.Equal(t, map[string]any{"count": uint64(1)}, event.Fields())

		_, err = usagetypes.ParseEvent[usagetypes.DCManagedAgentsV1]([]byte(`{"count": "invalid"}`))
		require.ErrorContains(t, err, "unmarshal usagetypes.DCManagedAgentsV1 event")

		_, err = usagetypes.ParseEvent[usagetypes.DCManagedAgentsV1]([]byte(`{}`))
		require.ErrorContains(t, err, "invalid usagetypes.DCManagedAgentsV1 event: count must be greater than 0")
	})
}

func TestParseEventWithType(t *testing.T) {
	t.Parallel()

	t.Run("UnknownEvent", func(t *testing.T) {
		t.Parallel()
		_, err := usagetypes.ParseEventWithType(usagetypes.UsageEventType("fake"), []byte(`{}`))
		require.ErrorContains(t, err, "unknown event type: fake")
	})

	t.Run("DCManagedAgentsV1", func(t *testing.T) {
		t.Parallel()

		eventType := usagetypes.UsageEventTypeDCManagedAgentsV1
		event, err := usagetypes.ParseEventWithType(eventType, []byte(`{"count": 1}`))
		require.NoError(t, err)
		require.Equal(t, usagetypes.DCManagedAgentsV1{Count: 1}, event)
		require.Equal(t, eventType, event.EventType())
		require.Equal(t, map[string]any{"count": uint64(1)}, event.Fields())
	})
}
