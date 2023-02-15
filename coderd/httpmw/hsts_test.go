package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/httpmw"
)

func TestHSTS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name    string
		MaxAge  int
		Options []string

		wantErr      bool
		expectHeader string
	}{
		{
			Name:    "Empty",
			MaxAge:  0,
			Options: nil,
		},
		{
			Name:    "NoAge",
			MaxAge:  0,
			Options: []string{"includeSubDomains"},
		},
		{
			Name:    "NegativeAge",
			MaxAge:  -100,
			Options: []string{"includeSubDomains"},
		},
		{
			Name:         "Age",
			MaxAge:       1000,
			Options:      []string{},
			expectHeader: "max-age=1000",
		},
		{
			Name:   "AgeSubDomains",
			MaxAge: 1000,
			// Mess with casing
			Options:      []string{"INCLUDESUBDOMAINS"},
			expectHeader: "max-age=1000; includeSubDomains",
		},
		{
			Name:         "AgePreload",
			MaxAge:       1000,
			Options:      []string{"Preload"},
			expectHeader: "max-age=1000; preload",
		},
		{
			Name:         "AllOptions",
			MaxAge:       1000,
			Options:      []string{"preload", "includeSubDomains"},
			expectHeader: "max-age=1000; preload; includeSubDomains",
		},

		// Error values
		{
			Name:    "BadOption",
			MaxAge:  100,
			Options: []string{"not-valid"},
			wantErr: true,
		},
		{
			Name:    "BadOptions",
			MaxAge:  100,
			Options: []string{"includeSubDomains", "not-valid", "still-not-valid"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			cfg, err := httpmw.HSTSConfigOptions(tt.MaxAge, tt.Options)
			if tt.wantErr {
				require.Error(t, err, "Expect error, HSTS(%v, %v)", tt.MaxAge, tt.Options)
				return
			}
			require.NoError(t, err, "Expect no error, HSTS(%v, %v)", tt.MaxAge, tt.Options)

			got := httpmw.HSTS(handler, cfg)
			req := httptest.NewRequest("GET", "/", nil)
			res := httptest.NewRecorder()
			got.ServeHTTP(res, req)

			require.Equal(t, tt.expectHeader, res.Header().Get("Strict-Transport-Security"), "expected header value")
		})
	}
}
