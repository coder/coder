package coderd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
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
		tt := tt

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
