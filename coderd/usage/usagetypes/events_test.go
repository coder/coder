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
		var event usagetypes.DCManagedAgentsV1
		err := usagetypes.ParseEvent([]byte(`{"count": 1, "extra": "field"}`), &event)
		require.ErrorContains(t, err, "unmarshal *usagetypes.DCManagedAgentsV1 event")
	})

	t.Run("ExtraData", func(t *testing.T) {
		t.Parallel()
		var event usagetypes.DCManagedAgentsV1
		err := usagetypes.ParseEvent([]byte(`{"count": 1}{"count": 2}`), &event)
		require.ErrorContains(t, err, "extra data after *usagetypes.DCManagedAgentsV1 event")
	})

	t.Run("DCManagedAgentsV1", func(t *testing.T) {
		t.Parallel()

		var event usagetypes.DCManagedAgentsV1
		err := usagetypes.ParseEvent([]byte(`{"count": 1}`), &event)
		require.NoError(t, err)
		require.Equal(t, usagetypes.DCManagedAgentsV1{Count: 1}, event)
		require.Equal(t, map[string]any{"count": uint64(1)}, event.Fields())

		event = usagetypes.DCManagedAgentsV1{}
		err = usagetypes.ParseEvent([]byte(`{"count": "invalid"}`), &event)
		require.ErrorContains(t, err, "unmarshal *usagetypes.DCManagedAgentsV1 event")

		event = usagetypes.DCManagedAgentsV1{}
		err = usagetypes.ParseEvent([]byte(`{}`), &event)
		require.ErrorContains(t, err, "invalid *usagetypes.DCManagedAgentsV1 event: count must be greater than 0")
	})
}

func TestParseEventWithType(t *testing.T) {
	t.Parallel()

	t.Run("UnknownEvent", func(t *testing.T) {
		t.Parallel()
		_, err := usagetypes.ParseEventWithType(usagetypes.UsageEventType("fake"), []byte(`{}`))
		var unknownEventTypeError usagetypes.UnknownEventTypeError
		require.ErrorAs(t, err, &unknownEventTypeError)
		require.Equal(t, "fake", unknownEventTypeError.EventType)
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
