package automations_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/automations"
)

func TestMatchFilter(t *testing.T) {
	t.Parallel()

	payload := `{"action":"opened","repository":{"full_name":"coder/coder"},"pull_request":{"number":42}}`

	tests := []struct {
		name   string
		filter json.RawMessage
		want   bool
	}{
		{"nil filter matches everything", nil, true},
		{"null filter matches everything", json.RawMessage(`null`), true},
		{"empty filter matches everything", json.RawMessage(`{}`), true},
		{"single match", json.RawMessage(`{"action":"opened"}`), true},
		{"nested match", json.RawMessage(`{"repository.full_name":"coder/coder"}`), true},
		{"multiple conditions all match", json.RawMessage(`{"action":"opened","repository.full_name":"coder/coder"}`), true},
		{"value mismatch", json.RawMessage(`{"action":"closed"}`), false},
		{"path does not exist", json.RawMessage(`{"nonexistent":"value"}`), false},
		{"one of two conditions fails", json.RawMessage(`{"action":"opened","repository.full_name":"other/repo"}`), false},
		{"numeric match", json.RawMessage(`{"pull_request.number":42}`), true},
		{"numeric mismatch", json.RawMessage(`{"pull_request.number":99}`), false},
		{"invalid filter json", json.RawMessage(`not json`), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := automations.MatchFilter(payload, tt.filter)
			require.Equal(t, tt.want, got)
		})
	}
}
