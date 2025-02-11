package coderd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStripSlashesMW(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		inputPath string
		wantPath  string
	}{
		{"No changes", "/api/v1/buildinfo", "/api/v1/buildinfo"},
		{"Single trailing slash", "/api/v2/buildinfo/", "/api/v2/buildinfo"},
		{"Double slashes", "/api//v2//buildinfo", "/api/v2/buildinfo"},
		{"Triple slashes", "/api///v2///buildinfo///", "/api/v2/buildinfo"},
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

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("GET", tt.inputPath, nil)
			rec := httptest.NewRecorder()

			stripSlashesMW(handler).ServeHTTP(rec, req)

			if req.URL.Path != tt.wantPath {
				t.Errorf("stripSlashesMW got path = %q, want %q", req.URL.Path, tt.wantPath)
			}
		})
	}
}
