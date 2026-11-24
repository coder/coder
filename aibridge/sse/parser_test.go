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
		want   map[string][]sse.Event
	}{
		{
			name:   "comment_only_keepalives_skipped",
			stream: ": OPENROUTER PROCESSING\n\n: OPENROUTER PROCESSING\n\ndata: {\"ok\":true}\n\n",
			want: map[string][]sse.Event{
				sse.EventTypeMessage: {{Type: sse.EventTypeMessage, Data: `{"ok":true}`}},
			},
		},
		{
			name:   "repeated_blank_lines_skipped",
			stream: "data: a\n\n\n\ndata: b\n\n\n",
			want: map[string][]sse.Event{
				sse.EventTypeMessage: {
					{Type: sse.EventTypeMessage, Data: "a"},
					{Type: sse.EventTypeMessage, Data: "b"},
				},
			},
		},
		{
			name:   "event_without_data_skipped",
			stream: "event: ping\n\ndata: a\n\n",
			want: map[string][]sse.Event{
				sse.EventTypeMessage: {{Type: sse.EventTypeMessage, Data: "a"}},
			},
		},
		{
			name:   "empty_data_field_dispatched",
			stream: "event: ping\ndata:\n\n",
			want: map[string][]sse.Event{
				sse.EventTypePing: {{Type: sse.EventTypePing}},
			},
		},
		{
			name:   "multiline_data_joined",
			stream: "event: content_block_delta\r\nid: 7\r\ndata: first\r\ndata: second\r\n\r\n",
			want: map[string][]sse.Event{
				"content_block_delta": {{Type: "content_block_delta", ID: "7", Data: "first\nsecond"}},
			},
		},
		{
			name:   "line_over_scanner_default_parsed",
			stream: "data: " + longData + "\n\ndata: after\n\n",
			want: map[string][]sse.Event{
				sse.EventTypeMessage: {
					{Type: sse.EventTypeMessage, Data: longData},
					{Type: sse.EventTypeMessage, Data: "after"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := sse.NewParser()
			require.NoError(t, p.Parse(strings.NewReader(tt.stream)))
			require.Equal(t, tt.want, p.AllEvents())
		})
	}
}
