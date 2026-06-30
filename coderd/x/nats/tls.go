package nats

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/quartz"
)

const (
	// leafCertValidity is the lifetime of an ephemeral cluster leaf
	// certificate. Leaves are re-minted before expiry and whenever the active
	// CA rotates, so this can be well under the CA's own validity window
	// (cryptokeys.NATSCALeafValidity).
	leafCertValidity = 24 * time.Hour
	// leafRenewBefore re-mints the leaf this long before it expires so an
	// in-flight handshake never races expiry.
	leafRenewBefore = time.Hour
	// clusterTLSTimeout is the route TLS handshake timeout. NATS defaults to a
	// tight 2s, which is flaky under load and in CI.
	clusterTLSTimeout = 10 * time.Second
	// leafSerialBits is the entropy of a leaf certificate serial number.
	leafSerialBits = 128
	// clockSkewToleranceTLS backdates a leaf's NotBefore so a peer with a
	// mildly skewed clock still accepts a freshly minted leaf.
	clockSkewToleranceTLS = time.Hour
)

// ClusterCAKeycache is the read-only view of the nats_ca signing key cache that
// the cluster TLS layer needs. cryptokeys.SigningKeycache satisfies it, so the
// nats_ca cache is passed straight through with no adapter.
//
// SigningKey returns the active CA used to mint this replica's leaf;
// VerifyingKey returns a specific CA by its crypto_keys sequence, used to
// verify a peer leaf that was minted under that (possibly older) CA during a
// rotation overlap. Both return a *cryptokeys.NATSCA.
type ClusterCAKeycache interface {
	SigningKey(ctx context.Context) (id string, key interface{}, err error)
	VerifyingKey(ctx context.Context, id string) (key interface{}, err error)
}

// clusterTLS builds the cluster route *tls.Config. Certificate selection and
// peer verification are tls.Config callbacks that consult the CA cache on each
// use, so a CA rotation is tracked without restarting or reloading the server.
type clusterTLS struct {
	ctx    context.Context
	logger slog.Logger
	clock  quartz.Clock

	mu sync.Mutex
	// ca and ip are swapped together by setClusterCA: under the default noop
	// cache no leaf can be minted (so no route forms), and the real cache plus
	// this replica's relay IP are installed once cluster mTLS is enabled.
	ca ClusterCAKeycache
	ip net.IP
	// leaf is the cached leaf certificate. leafSeq is the active CA sequence it
	// was minted under; a change means the CA rotated and the leaf is stale.
	leaf    *tls.Certificate
	leafSeq string
	// verifyPools caches the root pool used to verify a peer leaf, keyed by the
	// CA sequence stamped in the leaf. A CA cert is immutable for a given
	// sequence, so the pool is built once and reused across handshakes. Expired
	// entries are pruned on insert to bound the map across rotations.
	verifyPools map[string]cachedVerifyPool
}

// cachedVerifyPool is a verify root pool plus the NotAfter of the CA cert it
// holds; the entry is dropped once the clock passes notAfter.
type cachedVerifyPool struct {
	pool     *x509.CertPool
	notAfter time.Time
}

func newClusterTLS(ctx context.Context, logger slog.Logger, clock quartz.Clock, ca ClusterCAKeycache, ip net.IP) *clusterTLS {
	if clock == nil {
		clock = quartz.NewReal()
	}
	return &clusterTLS{
		ctx:    ctx,
		logger: logger.Named("cluster_tls"),
		clock:  clock,
		ca:     ca,
		ip:     ip,
	}
}

// setClusterCA swaps the CA cache and this replica's leaf IP SAN. Because the
// tls.Config callbacks read these on each handshake, the swap takes effect
// without a server restart or route reload: installing the real cache lets
// routes negotiate mTLS, and reverting to a noop cache makes leaf minting fail
// so no new route can form. A swap clears the cached leaf so the next handshake
// re-mints under the new CA/IP.
func (t *clusterTLS) setClusterCA(ca ClusterCAKeycache, ip net.IP) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ca = ca
	t.ip = ip
	t.leaf = nil
	t.leafSeq = ""
	// Verify pools follow the CA source: drop them so stale roots are not
	// reused after a swap to a noop or different CA.
	t.verifyPools = nil
}

// caCache returns the current CA cache under lock so callers do not hold the
// lock across cache I/O.
func (t *clusterTLS) caCache() ClusterCAKeycache {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ca
}

// tlsConfig returns the *tls.Config for the embedded server's cluster route
// listener. The same config is used by NATS for both accepting inbound routes
// (TLS server) and soliciting outbound routes (TLS client), so it sets both
// GetCertificate and GetClientCertificate.
//
// Verification is done in VerifyConnection against the CA fetched fresh from
// the cache, not against a static RootCAs/ClientCAs pool that cannot follow a
// rotating CA. InsecureSkipVerify disables Go's default static-root check on
// the dialing side ONLY so verifyConnection can run instead; it does not make
// the connection unauthenticated. Every connection is still mutually verified
// (ClientAuth requires a peer certificate) against live CA material.
//
// GetConfigForClient runs only when accepting a route (TLS server side), where
// the dialing peer's source IP is available on the underlying connection. It
// returns a per-connection config whose VerifyConnection additionally requires
// the peer leaf's IP SAN to match that source IP, binding the certificate to
// the network origin. The dialing side has no equivalent hook (Go does not
// expose the connection in client-certificate callbacks), so it relies on the
// base VerifyConnection: chain + membership against the known peer set.
func (t *clusterTLS) tlsConfig() *tls.Config {
	return &tls.Config{
		MinVersion:           tls.VersionTLS13,
		GetCertificate:       func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return t.currentLeaf() },
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return t.currentLeaf() },
		ClientAuth:           tls.RequireAnyClientCert,
		//nolint:gosec // Not insecure: verifyConnection performs full chain
		// verification against the live CA cache. Go's static RootCAs cannot
		// track a rotating CA, so default verification is replaced, not removed.
		InsecureSkipVerify: true,
		VerifyConnection:   func(cs tls.ConnectionState) error { return t.verifyLogged(cs, nil) },
		GetConfigForClient: t.configForClient,
	}
}

// verifyLogged runs verify and debug-logs a rejection. The embedded NATS server
// runs with NoLog set, so it swallows its own TLS handshake errors; logging here
// gives deployments a way to see why a cluster peer was rejected.
func (t *clusterTLS) verifyLogged(cs tls.ConnectionState, sourceIP net.IP) error {
	err := t.verify(cs, sourceIP)
	if err != nil {
		t.logger.Debug(t.ctx, "rejected nats cluster peer certificate", slog.Error(err))
	}
	return err
}

// configForClient builds the per-connection config used when accepting a route.
// It captures the dialing peer's source IP from the underlying connection so
// VerifyConnection can require the peer leaf's IP SAN to match it. NATS calls
// this on each inbound handshake, so a fresh config is allocated per accepted
// connection; that is fine at cluster-route cardinality (a handful of peers).
func (t *clusterTLS) configForClient(chi *tls.ClientHelloInfo) (*tls.Config, error) {
	var sourceIP net.IP
	if chi.Conn != nil {
		if remote := chi.Conn.RemoteAddr(); remote != nil {
			if host, _, err := net.SplitHostPort(remote.String()); err == nil {
				sourceIP = net.ParseIP(host)
			}
		}
	}
	cfg := &tls.Config{
		MinVersion:     tls.VersionTLS13,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return t.currentLeaf() },
		ClientAuth:     tls.RequireAnyClientCert,
		//nolint:gosec // See tlsConfig: verification is performed in VerifyConnection.
		InsecureSkipVerify: true,
		VerifyConnection:   func(cs tls.ConnectionState) error { return t.verifyLogged(cs, sourceIP) },
	}
	return cfg, nil
}

// currentLeaf returns the cached leaf, re-minting it when it is missing, near
// expiry, or signed by a CA that is no longer the active one (a rotation).
//
// The whole method holds t.mu so the CA cache, IP, and cached leaf are read as
// a consistent set: a concurrent setClusterCA cannot swap the CA out from under
// the IP we mint with. The lock is therefore held across the SigningKey lookup
// and the (rare) mint. Mints happen only at startup, when the leaf nears expiry
// (~daily), and on CA rotation, so the keygen+sign cost on the lock is
// acceptable; the SigningKey lookup is normally an in-memory cache hit.
func (t *clusterTLS) currentLeaf() (*tls.Certificate, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	id, key, err := t.ca.SigningKey(t.ctx)
	if err != nil {
		return nil, xerrors.Errorf("get signing CA: %w", err)
	}
	ca, ok := key.(*cryptokeys.NATSCA)
	if !ok {
		return nil, xerrors.Errorf("unexpected signing key type %T", key)
	}

	now := t.clock.Now()
	if t.leaf != nil && t.leafSeq == id && now.Before(t.leaf.Leaf.NotAfter.Add(-leafRenewBefore)) {
		return t.leaf, nil
	}

	leaf, err := mintLeaf(ca, t.ip, now)
	if err != nil {
		return nil, err
	}
	t.leaf = leaf
	t.leafSeq = id
	t.logger.Debug(t.ctx, "minted nats cluster leaf", slog.F("ca_sequence", id))
	return leaf, nil
}

// mintLeaf creates an ephemeral leaf certificate signed by the active CA. The
// signing CA's sequence is stamped into the leaf's Subject SerialNumber so a
// verifying peer can look up exactly that CA (see verifyConnection), and the
// replica's relay IP is embedded as an IP SAN so a dialing peer can confirm it
// reached the host it intended. The leaf is usable as both a TLS server and
// client certificate because each replica both accepts and dials cluster
// routes.
func mintLeaf(ca *cryptokeys.NATSCA, ip net.IP, now time.Time) (*tls.Certificate, error) {
	if len(ip) == 0 {
		return nil, xerrors.New("leaf IP SAN is required")
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, xerrors.Errorf("generate leaf key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), leafSerialBits))
	if err != nil {
		return nil, xerrors.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "coder-nats-cluster-leaf",
			// SerialNumber carries the sequence of the CA that signed this
			// leaf, letting a verifier fetch exactly that CA from its cache.
			SerialNumber: strconv.FormatInt(int64(ca.Sequence), 10),
		},
		IPAddresses:           []net.IP{ip},
		NotBefore:             now.Add(-clockSkewToleranceTLS),
		NotAfter:              now.Add(leafCertValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, template, ca.Cert, &leafKey.PublicKey, ca.Key)
	if err != nil {
		return nil, xerrors.Errorf("create leaf certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return nil, xerrors.Errorf("parse leaf certificate: %w", err)
	}

	return &tls.Certificate{
		Certificate: [][]byte{leafDER},
		PrivateKey:  leafKey,
		Leaf:        leaf,
	}, nil
}

// verify verifies a peer's leaf certificate. It reads the signing CA sequence
// the peer stamped into its leaf, fetches that exact CA from the cache, and
// confirms the leaf chains to it. Using the stamped sequence is not a trust
// decision: the leaf must still chain to OUR trusted copy of that CA, and a CA
// that has been retired is no longer returned by the cache, so leaves from a
// deleted CA are rejected.
//
// It then enforces source binding: when sourceIP is set (the accept side, where
// the dialing peer's connection address is available), the leaf must carry that
// source IP as an IP SAN, binding the certificate to the network origin. Go's
// default hostname verification, which InsecureSkipVerify disables, cannot do
// this because Go does not populate cs.ServerName for IP-based routes. On the
// dial side sourceIP is nil (Go does not expose the connection in the
// client-certificate callbacks), so only the chain is verified there.
func (t *clusterTLS) verify(cs tls.ConnectionState, sourceIP net.IP) error {
	if len(cs.PeerCertificates) == 0 {
		return xerrors.New("no peer certificate presented")
	}
	leaf := cs.PeerCertificates[0]

	seq := leaf.Subject.SerialNumber
	if seq == "" {
		return xerrors.New("peer leaf missing signing CA sequence")
	}

	key, err := t.caCache().VerifyingKey(t.ctx, seq)
	if err != nil {
		return xerrors.Errorf("get CA for sequence %q: %w", seq, err)
	}
	ca, ok := key.(*cryptokeys.NATSCA)
	if !ok {
		return xerrors.Errorf("unexpected verifying key type %T", key)
	}

	// Leaves carry both ServerAuth and ClientAuth, since each replica is both a
	// route server and client. Requiring those specific usages rejects a leaf
	// with some unexpected EKU rather than accepting any usage.
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:     t.verifyPool(seq, ca.Cert),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}); err != nil {
		return xerrors.Errorf("verify peer leaf against CA sequence %q: %w", seq, err)
	}

	// On the accept side, confirm the leaf's IP SAN matches the address the
	// peer actually connected from.
	if len(sourceIP) != 0 && !leafHasIP(leaf, sourceIP) {
		return xerrors.Errorf("peer leaf IP SANs %v do not match source IP %s", leaf.IPAddresses, sourceIP)
	}
	return nil
}

// leafHasIP reports whether the leaf carries ip as an IP SAN.
func leafHasIP(leaf *x509.Certificate, ip net.IP) bool {
	for _, san := range leaf.IPAddresses {
		if san.Equal(ip) {
			return true
		}
	}
	return false
}

// verifyPool returns the root pool used to verify a peer leaf minted under the
// given CA sequence, building it once and caching it for reuse. It is called
// from verify on every route handshake; cluster routes are long-lived, so a
// handshake is a rare event, and the common case here is a cache hit (a single
// map lookup).
//
// A miss occurs only the first time a sequence is seen (startup, and once per
// CA rotation), which is the only moment the map can grow, so pruning of expired
// entries is attached to the miss path rather than run on every handshake. An
// entry is dropped once the clock passes the CA cert's NotAfter: no valid leaf
// can chain to an expired CA, and the CA outlives every leaf it signed, so this
// is always safe and bounds the map across rotations.
func (t *clusterTLS) verifyPool(seq string, cert *x509.Certificate) *x509.CertPool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if cp, ok := t.verifyPools[seq]; ok {
		return cp.pool
	}

	now := t.clock.Now()
	for s, cp := range t.verifyPools {
		if now.After(cp.notAfter) {
			delete(t.verifyPools, s)
		}
	}

	pool := x509.NewCertPool()
	pool.AddCert(cert)
	if t.verifyPools == nil {
		t.verifyPools = map[string]cachedVerifyPool{}
	}
	t.verifyPools[seq] = cachedVerifyPool{pool: pool, notAfter: cert.NotAfter}
	return pool
}
