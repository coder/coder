package xhttp_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/util/xhttp"
	"github.com/coder/coder/v2/testutil"
)

func TestHandlerTransport(t *testing.T) {
	t.Parallel()

	roundTrip := func(ctx context.Context, t *testing.T, h http.Handler) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://handler/path", nil)
		require.NoError(t, err)
		req.Header.Set("X-Request", "yes")
		resp, err := xhttp.HandlerTransport(h).RoundTrip(req)
		require.NoError(t, err)
		return resp
	}

	t.Run("PassesRequestAndResponse", func(t *testing.T) {
		t.Parallel()

		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Response", r.Header.Get("X-Request"))
			w.WriteHeader(http.StatusTeapot)
			_, _ = io.WriteString(w, r.URL.Path)
		})

		resp := roundTrip(testutil.Context(t, testutil.WaitShort), t, h)
		defer resp.Body.Close()
		require.Equal(t, http.StatusTeapot, resp.StatusCode)
		require.Equal(t, "418 I'm a teapot", resp.Status)
		require.Equal(t, "yes", resp.Header.Get("X-Response"))
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, "/path", string(body))
	})

	// Chunks must arrive before the handler returns. SSE depends on it.
	t.Run("Streams", func(t *testing.T) {
		t.Parallel()

		const chunks = 3
		released := make([]chan struct{}, chunks)
		for i := range released {
			released[i] = make(chan struct{})
		}
		h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			if !assert.True(t, ok, "ResponseWriter must implement http.Flusher") {
				return
			}
			for i := range chunks {
				<-released[i]
				_, _ = fmt.Fprintf(w, "chunk-%d\n", i)
				flusher.Flush()
			}
		})

		resp := roundTrip(testutil.Context(t, testutil.WaitShort), t, h)
		defer resp.Body.Close()
		br := bufio.NewReader(resp.Body)
		for i := range chunks {
			close(released[i])
			line, err := br.ReadString('\n')
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf("chunk-%d\n", i), line)
		}
	})

	// A canceled request must end the body read even when the handler
	// ignores its context.
	t.Run("CancelClosesBody", func(t *testing.T) {
		t.Parallel()

		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			<-release
		})

		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		resp := roundTrip(ctx, t, h)
		defer resp.Body.Close()
		cancel()
		_, err := io.ReadAll(resp.Body)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("CancelBeforeHeaders", func(t *testing.T) {
		t.Parallel()

		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			<-release
		})

		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://handler/", nil)
		require.NoError(t, err)
		resp, err := xhttp.HandlerTransport(h).RoundTrip(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("HandlerPanic", func(t *testing.T) {
		t.Parallel()

		h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("boom")
		})

		resp := roundTrip(testutil.Context(t, testutil.WaitShort), t, h)
		defer resp.Body.Close()
		require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		_, err := io.ReadAll(resp.Body)
		require.ErrorContains(t, err, "handler panicked: boom")
	})

	t.Run("ReturnsWithoutWriting", func(t *testing.T) {
		t.Parallel()

		h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Response", "yes")
		})

		resp := roundTrip(testutil.Context(t, testutil.WaitShort), t, h)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "yes", resp.Header.Get("X-Response"))
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Empty(t, body)
	})

	t.Run("EmptyWritesDoNotBlock", func(t *testing.T) {
		t.Parallel()

		for _, payload := range [][]byte{nil, {}} {
			t.Run(fmt.Sprintf("Nil=%t", payload == nil), func(t *testing.T) {
				t.Parallel()

				h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					for _, chunk := range []string{"first\n", "second\n", ""} {
						for range 128 {
							if _, err := w.Write(payload); err != nil {
								return
							}
						}
						if _, err := io.WriteString(w, chunk); err != nil {
							return
						}
					}
				})

				resp := roundTrip(testutil.Context(t, testutil.WaitShort), t, h)
				defer resp.Body.Close()
				br := bufio.NewReader(resp.Body)
				for _, want := range []string{"first\n", "second\n"} {
					line, err := br.ReadString('\n')
					require.NoError(t, err)
					require.Equal(t, want, line)
				}
				line, err := br.ReadString('\n')
				require.ErrorIs(t, err, io.EOF)
				require.Empty(t, line)
			})
		}
	})

	t.Run("ConcurrentRequests", func(t *testing.T) {
		t.Parallel()

		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(w, r.Body)
		})
		rt := xhttp.HandlerTransport(h)
		ctx := testutil.Context(t, testutil.WaitShort)

		var eg errgroup.Group
		for i := range 16 {
			eg.Go(func() error {
				payload := fmt.Sprintf("payload-%d", i)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://handler/", strings.NewReader(payload))
				if err != nil {
					return err
				}
				resp, err := rt.RoundTrip(req)
				if err != nil {
					return err
				}
				defer resp.Body.Close()
				got, err := io.ReadAll(resp.Body)
				if err != nil {
					return err
				}
				if string(got) != payload {
					return xerrors.Errorf("want %q, got %q", payload, got)
				}
				return nil
			})
		}
		require.NoError(t, eg.Wait())
	})
}
