package coderd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd"
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

// TestChatDaemonPublishDiffStatusChangeFunc regression-tests the guard
// that fixes a nil-pointer panic: a Go method value on a nil pointer
// receiver is itself non-nil, so binding
// chatDaemon.PublishDiffStatusChange unconditionally would defeat
// gitsync.Worker's own nil check on its callback and panic when the
// worker later invoked it. This test proves the returned func is a
// true nil (not a non-nil method value wrapping a nil receiver) when
// chatDaemon is nil, and a working, callable func when it isn't.
func TestChatDaemonPublishDiffStatusChangeFunc(t *testing.T) {
	t.Parallel()

	t.Run("NilChatDaemon", func(t *testing.T) {
		t.Parallel()
		fn := chatDaemonPublishDiffStatusChangeFunc(nil)
		require.Nil(t, fn, "func value must be a true nil, not a bound method on a nil receiver")
	})

	t.Run("NonNilChatDaemon", func(t *testing.T) {
		t.Parallel()
		daemon := &chatd.Server{}
		fn := chatDaemonPublishDiffStatusChangeFunc(daemon)
		require.NotNil(t, fn)
	})
}
