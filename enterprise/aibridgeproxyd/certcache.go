package aibridgeproxyd

import (
	"crypto/tls"
	"sync"
)

// CertCache implements goproxy.CertStorage to cache generated leaf certificates
// in memory. Certificate generation is expensive (RSA key generation + signing),
// so caching avoids repeated generation for the same hostname during MITM.
type CertCache struct {
	mu    sync.RWMutex
	certs map[string]*tls.Certificate
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
func (c *CertCache) Fetch(hostname string, genFunc func() (*tls.Certificate, error)) (*tls.Certificate, error) {
	c.mu.RLock()
	cert, ok := c.certs[hostname]
	c.mu.RUnlock()
	if ok {
		return cert, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock to avoid duplicate generation
	// if another goroutine generated the cert between releasing the read lock
	// and acquiring the write lock.
	if cert, ok := c.certs[hostname]; ok {
		return cert, nil
	}

	cert, err := genFunc()
	if err != nil {
		return nil, err
	}
	c.certs[hostname] = cert
	return cert, nil
}
