package aibridgeproxyd

import (
	"crypto/tls"
	"sync"

	"golang.org/x/xerrors"
	"tailscale.com/util/singleflight"
)

// CertCache implements goproxy.CertStorage to cache generated leaf certificates
// in memory. Certificate generation is expensive (RSA key generation + signing),
// so caching avoids repeated generation for the same hostname during MITM.
type CertCache struct {
	mu           sync.RWMutex
	certs        map[string]*tls.Certificate
	singleFlight singleflight.Group[string, *tls.Certificate]
}

// NewCertCache creates a new certificate cache that maps hostnames to their
// generated TLS certificates.
func NewCertCache() *CertCache {
	return &CertCache{
		certs: make(map[string]*tls.Certificate),
	}
}

// Fetch retrieves a cached certificate for the given hostname, or generates
// and caches a new one using the provided generator function.
//
// Uses singleflight to ensure concurrent requests for the same hostname share
// a single in-flight generation rather than waiting on a mutex. This means only
// one goroutine generates the certificate while others wait on the result directly.
func (c *CertCache) Fetch(hostname string, genFunc func() (*tls.Certificate, error)) (*tls.Certificate, error) {
	// Cache hit: check cache with read lock.
	c.mu.RLock()
	cert, ok := c.certs[hostname]
	c.mu.RUnlock()
	if ok {
		return cert, nil
	}

	// Cache miss: use singleflight to ensure only one goroutine generates
	// the certificate for a given hostname, even under concurrent requests.
	cert, err, _ := c.singleFlight.Do(hostname, func() (*tls.Certificate, error) {
		// Double-check cache inside singleflight in case another call
		// already populated it.
		c.mu.RLock()
		if cert, ok := c.certs[hostname]; ok {
			c.mu.RUnlock()
			return cert, nil
		}
		c.mu.RUnlock()

		cert, err := genFunc()
		if err != nil {
			return nil, err
		}
		if cert == nil {
			return nil, xerrors.New("generator function returned nil certificate")
		}

		c.mu.Lock()
		c.certs[hostname] = cert
		c.mu.Unlock()

		return cert, nil
	})

	return cert, err
}
