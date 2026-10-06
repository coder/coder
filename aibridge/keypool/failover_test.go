package keypool_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/quartz"
)

// errFakeRoundTripperCalled is returned by fakeRoundTripper if it
// ever gets invoked. The constructor identity tests should never
// trigger a RoundTrip call.
var errFakeRoundTripperCalled = xerrors.New("fakeRoundTripper should not be invoked")

// fakeRoundTripper is a no-op http.RoundTripper used to check
// constructor identity in tests.
type fakeRoundTripper struct{}

func (*fakeRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errFakeRoundTripperCalled
}

func TestNewKeyFailoverTransport(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New("test-provider", []string{"k0"}, quartz.NewMock(t), nil)
	require.NoError(t, err)

	tests := []struct {
		name string
		// Constructor input.
		config keypool.KeyFailoverConfig
		// Whether the constructor returns inner unchanged.
		expectSame bool
	}{
		{
			// Pool is nil: failover is disabled, inner is returned unchanged.
			name:       "pool_nil_returns_inner",
			config:     keypool.KeyFailoverConfig{},
			expectSame: true,
		},
		{
			// Pool is set: inner is wrapped in a key-failover transport.
			name:       "pool_set_returns_wrapper",
			config:     keypool.KeyFailoverConfig{Pool: pool},
			expectSame: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inner := &fakeRoundTripper{}
			got := keypool.NewKeyFailoverTransport(inner, tc.config)

			if tc.expectSame {
				assert.Same(t, inner, got)
			} else {
				assert.NotSame(t, inner, got)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// eofCountingReader counts how many times the wrapped reader is read to EOF.
type eofCountingReader struct {
	io.Reader
	eofs int
}

func (r *eofCountingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if errors.Is(err, io.EOF) {
		r.eofs++
	}
	return n, err
}

func TestKeyFailoverTransportReadsBodyOnce(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New("test", []string{"k0", "k1"}, quartz.NewMock(t), nil)
	require.NoError(t, err)
	var bodies []string
	rt := keypool.NewKeyFailoverTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		bodies = append(bodies, string(body))
		status := http.StatusOK
		if len(bodies) == 1 {
			status = http.StatusUnauthorized
		}
		return &http.Response{StatusCode: status, Body: http.NoBody}, nil
	}), keypool.KeyFailoverConfig{
		Pool:          pool,
		IsBYOK:        func(*http.Request) bool { return false },
		InjectAuthKey: func(*http.Header, string) {},
	})

	body := &eofCountingReader{Reader: strings.NewReader("payload")}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.test", body)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []string{"payload", "payload"}, bodies)
	require.Equal(t, 1, body.eofs)
}
