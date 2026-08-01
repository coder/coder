package coderd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripSlashesMW(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inputPath string
		wantPath  string
	}{
		{"No changes", "/api/v1/buildinfo", "/api/v1/buildinfo"},
		{"Double slashes", "/api//v2//buildinfo", "/api/v2/buildinfo"},
		{"Triple slashes", "/api///v2///buildinfo", "/api/v2/buildinfo"},
		{"Leading slashes", "///api/v2/buildinfo", "/api/v2/buildinfo"},
		{"Root path", "/", "/"},
		{"Double slashes root", "//", "/"},
		{"Only slashes", "/////", "/"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, tt := range tests {
		t.Run("chi/"+tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest("GET", tt.inputPath, nil)
			rec := httptest.NewRecorder()

			// given
			rctx := chi.NewRouteContext()
			rctx.RoutePath = tt.inputPath
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			// when
			singleSlashMW(handler).ServeHTTP(rec, req)
			updatedCtx := chi.RouteContext(req.Context())

			// then
			assert.Equal(t, tt.inputPath, req.URL.Path)
			assert.Equal(t, tt.wantPath, updatedCtx.RoutePath)
		})

		t.Run("stdlib/"+tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest("GET", tt.inputPath, nil)
			rec := httptest.NewRecorder()

			// when
			singleSlashMW(handler).ServeHTTP(rec, req)

			// then
			assert.Equal(t, tt.wantPath, req.URL.Path)
			assert.Nil(t, chi.RouteContext(req.Context()))
		})
	}
}

// TestChatDaemonPublishDiffStatusChangeFunc verifies that
// chatDaemonPublishDiffStatusChangeFunc returns a true nil, not a method
// value bound to a nil receiver, when chatDaemon is nil. See that function
// for why the distinction matters. The non-nil path is covered by
// coderd/exp_chats_test.go.
func TestChatDaemonPublishDiffStatusChangeFunc(t *testing.T) {
	t.Parallel()

	fn := chatDaemonPublishDiffStatusChangeFunc(nil)
	require.Nil(t, fn, "func value must be a true nil, not a bound method on a nil receiver")
}

func TestSentryIngestOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dsn     string
		want    string
		wantErr bool
	}{
		{
			name: "HTTPSWithKey",
			dsn:  "https://abc123@o450.ingest.sentry.example/42",
			want: "https://o450.ingest.sentry.example",
		},
		{
			name: "HTTPSelfHosted",
			dsn:  "http://key@sentry.internal:9000/1",
			want: "http://sentry.internal:9000",
		},
		{
			name:    "NonHTTPScheme",
			dsn:     "ftp://key@host/1",
			wantErr: true,
		},
		{
			name:    "NoHost",
			dsn:     "https:///1",
			wantErr: true,
		},
		{
			name:    "NotAURL",
			dsn:     "::not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := sentryIngestOrigin(tt.dsn)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
