package chatd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func testRelayTextEvent(text string) codersdk.ChatStreamEvent {
	return codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{
			Role: "assistant",
			Part: codersdk.ChatMessageText(text),
		},
	}
}

func testRelayTextEvents(texts ...string) []codersdk.ChatStreamEvent {
	events := make([]codersdk.ChatStreamEvent, 0, len(texts))
	for _, text := range texts {
		events = append(events, testRelayTextEvent(text))
	}
	return events
}

func testRelayTexts(events []codersdk.ChatStreamEvent) []string {
	texts := make([]string, 0, len(events))
	for _, event := range events {
		if event.Type == codersdk.ChatStreamEventTypeMessagePart &&
			event.MessagePart != nil {
			texts = append(texts, event.MessagePart.Part.Text)
		}
	}
	return texts
}

func testRelayMessagePart(text string) *codersdk.ChatStreamMessagePart {
	return &codersdk.ChatStreamMessagePart{
		Role: "assistant",
		Part: codersdk.ChatMessageText(text),
	}
}

func TestTrimRelaySnapshotOverlap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		recent   []codersdk.ChatStreamEvent
		snapshot []codersdk.ChatStreamEvent
		want     []string
	}{
		{
			name:     "empty recent returns full snapshot",
			snapshot: testRelayTextEvents("a", "b"),
			want:     []string{"a", "b"},
		},
		{
			name:   "empty snapshot stays empty",
			recent: testRelayTextEvents("a"),
			want:   []string{},
		},
		{
			name: "both empty stay empty",
			want: []string{},
		},
		{
			name:     "single element exact match trims one part",
			recent:   testRelayTextEvents("a"),
			snapshot: testRelayTextEvents("a", "b"),
			want:     []string{"b"},
		},
		{
			name:     "full overlap trims the whole snapshot",
			recent:   testRelayTextEvents("a", "b"),
			snapshot: testRelayTextEvents("a", "b"),
			want:     []string{},
		},
		{
			name:     "ambiguous overlap falls back to full snapshot",
			recent:   testRelayTextEvents("a", "b", "a"),
			snapshot: testRelayTextEvents("a", "b", "a", "c"),
			want:     []string{"a", "b", "a", "c"},
		},
		{
			name:     "unambiguous overlap trims matching prefix",
			recent:   testRelayTextEvents("x", "a", "b"),
			snapshot: testRelayTextEvents("a", "b", "c"),
			want:     []string{"c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := trimRelaySnapshotOverlap(tt.recent, tt.snapshot)
			require.Equal(t, tt.want, testRelayTexts(got))
		})
	}
}

func TestAppendRecentRelayPart(t *testing.T) {
	t.Parallel()

	recentAtCap := make([]codersdk.ChatStreamEvent, 0, relaySnapshotCap)
	wantAtCap := make([]string, 0, relaySnapshotCap)
	for i := range relaySnapshotCap {
		recentAtCap = append(recentAtCap, testRelayTextEvent(fmt.Sprintf("part-%03d", i)))
		if i > 0 {
			wantAtCap = append(wantAtCap, fmt.Sprintf("part-%03d", i))
		}
	}
	wantAtCap = append(wantAtCap, fmt.Sprintf("part-%03d", relaySnapshotCap))

	tests := []struct {
		name   string
		recent []codersdk.ChatStreamEvent
		event  codersdk.ChatStreamEvent
		want   []string
	}{
		{
			name:   "append under cap preserves order",
			recent: testRelayTextEvents("a", "b"),
			event:  testRelayTextEvent("c"),
			want:   []string{"a", "b", "c"},
		},
		{
			name:   "append at cap keeps most recent entries",
			recent: recentAtCap,
			event:  testRelayTextEvent(fmt.Sprintf("part-%03d", relaySnapshotCap)),
			want:   wantAtCap,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recent := append([]codersdk.ChatStreamEvent(nil), tt.recent...)
			got := appendRecentRelayPart(recent, tt.event)
			require.Equal(t, tt.want, testRelayTexts(got))
		})
	}
}

func TestRelayMessagePartEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		left  *codersdk.ChatStreamMessagePart
		right *codersdk.ChatStreamMessagePart
		want  bool
	}{
		{
			name: "both nil are equal",
			want: true,
		},
		{
			name:  "left nil is not equal",
			right: testRelayMessagePart("a"),
		},
		{
			name: "right nil is not equal",
			left: testRelayMessagePart("a"),
		},
		{
			name:  "identical payloads compare equal",
			left:  testRelayMessagePart("same"),
			right: testRelayMessagePart("same"),
			want:  true,
		},
		{
			name:  "different payloads compare not equal",
			left:  testRelayMessagePart("left"),
			right: testRelayMessagePart("right"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, relayMessagePartEqual(tt.left, tt.right))
		})
	}
}
