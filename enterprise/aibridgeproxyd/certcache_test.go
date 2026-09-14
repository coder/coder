package aibridgeproxyd_test

import (
	"crypto/tls"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/enterprise/aibridgeproxyd"
)

func TestCertCache_Fetch(t *testing.T) {
	t.Parallel()

	t.Run("CacheMiss", func(t *testing.T) {
		t.Parallel()

		cache := aibridgeproxyd.NewCertCache()
		expectedCert := &tls.Certificate{}
		genCalls := 0

		cert, err := cache.Fetch("example.com", func() (*tls.Certificate, error) {
			genCalls++
			return expectedCert, nil
		})

		require.NoError(t, err)
		require.Same(t, expectedCert, cert)
		require.Equal(t, 1, genCalls)
	})

	t.Run("CacheHit", func(t *testing.T) {
		t.Parallel()

		cache := aibridgeproxyd.NewCertCache()
		expectedCert := &tls.Certificate{}
		genCalls := 0

		gen := func() (*tls.Certificate, error) {
			genCalls++
			return expectedCert, nil
		}

		// First call: cache miss
		cert1, err := cache.Fetch("example.com", gen)
		require.NoError(t, err)
		require.Same(t, expectedCert, cert1)
		require.Equal(t, 1, genCalls)

		// Second call: cache hit, generator should not be called
		cert2, err := cache.Fetch("example.com", gen)
		require.NoError(t, err)
		require.Same(t, expectedCert, cert2)
		require.Equal(t, 1, genCalls)
	})

	t.Run("DifferentHostnames", func(t *testing.T) {
		t.Parallel()

		cache := aibridgeproxyd.NewCertCache()
		cert1 := &tls.Certificate{}
		cert2 := &tls.Certificate{}

		result1, err := cache.Fetch("example1.com", func() (*tls.Certificate, error) {
			return cert1, nil
		})
		require.NoError(t, err)
		require.Same(t, cert1, result1)

		result2, err := cache.Fetch("example2.com", func() (*tls.Certificate, error) {
			return cert2, nil
		})
		require.NoError(t, err)
		require.Same(t, cert2, result2)

		// Verify different hostnames have different certificates.
		require.NotSame(t, result1, result2)
	})

	t.Run("GeneratorError", func(t *testing.T) {
		t.Parallel()

		cache := aibridgeproxyd.NewCertCache()
		expectedErr := xerrors.New("generation failed")

		cert, err := cache.Fetch("example.com", func() (*tls.Certificate, error) {
			return nil, expectedErr
		})

		require.ErrorIs(t, err, expectedErr)
		require.Nil(t, cert)
	})

	t.Run("GeneratorReturnsNil", func(t *testing.T) {
		t.Parallel()

		cache := aibridgeproxyd.NewCertCache()

		cert, err := cache.Fetch("example.com", func() (*tls.Certificate, error) {
			//nolint:nilnil // Intentionally testing this edge case
			return nil, nil
		})

		require.ErrorContains(t, err, "generator function returned nil certificate")
		require.Nil(t, cert)
	})

	t.Run("ConcurrentFetchSameHostname", func(t *testing.T) {
		t.Parallel()

		cache := aibridgeproxyd.NewCertCache()
		expectedCert := &tls.Certificate{}
		var genCalls atomic.Int32

		const numGoroutines = 10
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		var fetchErrors atomic.Int32

		// Spawn multiple goroutines that all request the same hostname concurrently.
		for range numGoroutines {
			go func() {
				defer wg.Done()
				cert, err := cache.Fetch("example.com", func() (*tls.Certificate, error) {
					genCalls.Add(1)
					return expectedCert, nil
				})
				if err != nil || cert != expectedCert {
					fetchErrors.Add(1)
				}
			}()
		}
		wg.Wait()

		require.Equal(t, int32(0), fetchErrors.Load())

		// Generator should only be called once due to double-check locking.
		require.Equal(t, int32(1), genCalls.Load())
	})
}
