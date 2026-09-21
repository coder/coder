package httpmw_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/httpmw"
	"github.com/coder/coder/v2/testutil"
)

func TestRecoverErrAbortHandler(t *testing.T) {
	t.Parallel()

	sink := testutil.NewFakeSink(t)
	handlerErr := make(chan error, 1)
	handler := httpmw.Recover(sink.Logger())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte("partial response"))
		if err != nil {
			handlerErr <- err
			return
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			handlerErr <- err
			return
		}
		handlerErr <- nil
		panic(http.ErrAbortHandler)
	}))

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ctx := testutil.Context(t, testutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	res, err := server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })

	body, err := io.ReadAll(res.Body)
	require.NoError(t, testutil.TryReceive(ctx, t, handlerErr))
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "partial response", string(body))
	require.Empty(t, sink.Entries())
}

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
