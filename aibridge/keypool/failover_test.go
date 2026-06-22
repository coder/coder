package keypool_test

import (
	"net/http"
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
