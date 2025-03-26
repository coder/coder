package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/testutil"
)

func TestRecover(t *testing.T) {
	t.Parallel()

	handler := func(isPanic, _ bool) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPanic {
				panic("Oh no!")
			}

			w.WriteHeader(http.StatusOK)
		})
	}

	cases := []struct {
		Name   string
		Code   int
		Panic  bool
		Hijack bool
	}{
		{
			Name:   "OK",
			Code:   http.StatusOK,
			Panic:  false,
			Hijack: false,
		},
		{
			Name:   "Panic",
			Code:   http.StatusInternalServerError,
			Panic:  true,
			Hijack: false,
		},
		{
			Name:   "Hijack",
			Code:   0,
			Panic:  true,
			Hijack: true,
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			var (
				log = testutil.Logger(t)
				r   = httptest.NewRequest("GET", "/", nil)
				w   = &tracing.StatusWriter{
					ResponseWriter: httptest.NewRecorder(),
					Hijacked:       c.Hijack,
				}
			)

			httpmw.Recover(log)(handler(c.Panic, c.Hijack)).ServeHTTP(w, r)

			require.Equal(t, c.Code, w.Status)
		})
	}
}
