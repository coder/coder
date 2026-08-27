package aibridge

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsWebSocketUpgrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		connection string
		upgrade    string
		want       bool
	}{
		{name: "websocket upgrade", method: http.MethodGet, connection: "keep-alive, Upgrade", upgrade: "WebSocket", want: true},
		{name: "non-GET request", method: http.MethodPost, connection: "Upgrade", upgrade: "websocket", want: false},
		{name: "missing connection upgrade", method: http.MethodGet, connection: "keep-alive", upgrade: "websocket", want: false},
		{name: "different upgrade protocol", method: http.MethodGet, connection: "Upgrade", upgrade: "h2c", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), tc.method, "/", nil)
			require.NoError(t, err)
			req.Header.Set("Connection", tc.connection)
			req.Header.Set("Upgrade", tc.upgrade)

			assert.Equal(t, tc.want, isWebSocketUpgrade(req))
		})
	}
}
