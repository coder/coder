package sse_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/sse"
)

func TestParserParse(t *testing.T) {
	t.Parallel()

	longData := strings.Repeat("x", 200*1024)

	tests := []struct {
		name   string
		stream string
		want   []sse.Event
	}{
		{
			name:   "comment_only_keepalives_skipped",
			stream: ": OPENROUTER PROCESSING\n\n: OPENROUTER PROCESSING\n\ndata: {\"ok\":true}\n\n",
			want:   []sse.Event{{Type: sse.EventTypeMessage, Data: `{"ok":true}`}},
		},
		{
			name:   "repeated_blank_lines_skipped",
			stream: "data: a\n\n\n\ndata: b\n\n\n",
			want: []sse.Event{
				{Type: sse.EventTypeMessage, Data: "a"},
				{Type: sse.EventTypeMessage, Data: "b"},
			},
		},
		{
			name:   "event_without_data_skipped",
			stream: "event: ping\n\ndata: a\n\n",
			want:   []sse.Event{{Type: sse.EventTypeMessage, Data: "a"}},
		},
		{
			name:   "empty_data_field_dispatched",
			stream: "event: ping\ndata:\n\n",
			want:   []sse.Event{{Type: sse.EventTypePing}},
		},
		{
			name:   "multiline_data_joined",
			stream: "event: content_block_delta\r\nid: 7\r\ndata: first\r\ndata: second\r\n\r\n",
			want:   []sse.Event{{Type: "content_block_delta", ID: "7", Data: "first\nsecond"}},
		},
		{
			name:   "line_over_scanner_default_parsed",
			stream: "data: " + longData + "\n\ndata: after\n\n",
			want: []sse.Event{
				{Type: sse.EventTypeMessage, Data: longData},
				{Type: sse.EventTypeMessage, Data: "after"},
			},
		},
		{
			name:   "interleaved_types_keep_stream_order",
			stream: "event: start\ndata: 1\n\nevent: ping\ndata: 2\n\nevent: start\ndata: 3\n\n",
			want: []sse.Event{
				{Type: "start", Data: "1"},
				{Type: sse.EventTypePing, Data: "2"},
				{Type: "start", Data: "3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := sse.NewParser()
			require.NoError(t, p.Parse(strings.NewReader(tt.stream)))
			require.Equal(t, tt.want, p.Events())
		})
	}
}

// The by-type views keep stream order within each type.
func TestParserEventsByType(t *testing.T) {
	t.Parallel()

	p := sse.NewParser()
	require.NoError(t, p.Parse(strings.NewReader("data: a\n\nevent: ping\ndata: p\n\ndata: b\n\n")))
	messages := []sse.Event{
		{Type: sse.EventTypeMessage, Data: "a"},
		{Type: sse.EventTypeMessage, Data: "b"},
	}
	require.Equal(t, messages, p.MessageEvents())
	require.Equal(t, map[string][]sse.Event{
		sse.EventTypeMessage: messages,
		sse.EventTypePing:    {{Type: sse.EventTypePing, Data: "p"}},
	}, p.AllEvents())
}
