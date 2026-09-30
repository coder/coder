package extract_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/extract"
)

// eventLog is a ResponseExtraction that records the events it is fed.
type eventLog struct{ events []string }

func (l *eventLog) OnEvent(eventType string, raw []byte) {
	l.events = append(l.events, eventType+"="+string(raw))
}
func (*eventLog) ProcessBlocking(int, []byte) {}
func (*eventLog) Outcome() extract.Outcome    { return extract.Outcome{} }

func TestSSEStream(t *testing.T) {
	t.Parallel()

	oversized := strings.Repeat("x", extract.MaxEventBytes)

	cases := []struct {
		name   string
		writes []string
		want   []string
	}{
		{
			// The event is skipped whole, even when it arrives in several
			// writes: its second data line must not become an event.
			name: "oversized_event_skipped",
			writes: []string{
				"event: big\ndata: " + oversized[:len(oversized)/2],
				oversized[len(oversized)/2:] + "\ndata: smuggled\n\n",
				"event: next\ndata: ok\n\n",
			},
			want: []string{"next=ok"},
		},
		{
			// The event name counts toward the bound: a large name plus
			// data that fits alone still exceeds it.
			name: "oversized_event_name_skipped",
			writes: []string{
				"event: " + oversized[:len(oversized)/2] + "\ndata: " + oversized[:len(oversized)/2+8] + "\n\n",
				"event: next\ndata: ok\n\n",
			},
			want: []string{"next=ok"},
		},
		{
			// An event without its terminating blank line is not
			// dispatched when the stream ends.
			name:   "unterminated_event_dropped",
			writes: []string{"event: first\ndata: one\n\nevent: second\ndata: two\n"},
			want:   []string{"first=one"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			log := &eventLog{}
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			sse := extract.NewSSEStream(t.Context(), logger, log)
			for _, w := range tc.writes {
				n, err := sse.Write([]byte(w))
				require.NoError(t, err)
				require.Equal(t, len(w), n)
			}
			require.NoError(t, sse.Close())
			require.Equal(t, tc.want, log.events)
		})
	}
}
