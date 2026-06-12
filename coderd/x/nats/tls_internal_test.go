package nats

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// generateTestCA returns a self-signed CA certificate and its private
// key for signing test leaf certificates.
func generateTestCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-cluster-ca",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return caCert, caKey
}

func Test_buildClusterTLSConfig_Validation(t *testing.T) {
	t.Parallel()

	caCert, caKey := generateTestCA(t)

	testCases := []struct {
		name    string
		opts    ClusterTLSOptions
		errPart string
	}{
		{
			name: "MissingCACert",
			opts: ClusterTLSOptions{
				CAKey:   caKey,
				SANHost: "127.0.0.1",
			},
			errPart: "CA certificate is required",
		},
		{
			name: "MissingCAKey",
			opts: ClusterTLSOptions{
				CACert:  caCert,
				SANHost: "127.0.0.1",
			},
			errPart: "CA private key is required",
		},
		{
			name: "EmptySANHost",
			opts: ClusterTLSOptions{
				CACert: caCert,
				CAKey:  caKey,
			},
			errPart: "is not an IP address",
		},
		{
			name: "HostnameSANHost",
			opts: ClusterTLSOptions{
				CACert:  caCert,
				CAKey:   caKey,
				SANHost: "coderd-0.coderd.svc.cluster.local",
			},
			errPart: "is not an IP address",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := buildClusterTLSConfig(tc.opts)
			require.ErrorContains(t, err, tc.errPart)
			require.Nil(t, cfg)
		})
	}
}

func Test_buildClusterTLSConfig_Leaf(t *testing.T) {
	t.Parallel()

	caCert, caKey := generateTestCA(t)

	testCases := []struct {
		name    string
		sanHost string
	}{
		{name: "IPv4", sanHost: "10.0.1.5"},
		{name: "IPv6", sanHost: "fd12:3456:789a::1"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := buildClusterTLSConfig(ClusterTLSOptions{
				CACert:  caCert,
				CAKey:   caKey,
				SANHost: tc.sanHost,
			})
			require.NoError(t, err)

			require.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
			require.Equal(t, tls.RequireAndVerifyClientCert, cfg.ClientAuth)
			require.NotNil(t, cfg.RootCAs)
			require.NotNil(t, cfg.ClientCAs)
			require.False(t, cfg.InsecureSkipVerify)

			require.Len(t, cfg.Certificates, 1)
			leaf := cfg.Certificates[0].Leaf
			require.NotNil(t, leaf)

			// The leaf must verify against the CA pool for both route
			// roles: ServerAuth when accepting and ClientAuth when
			// dialing.
			wantIP := net.ParseIP(tc.sanHost)
			require.Len(t, leaf.IPAddresses, 1)
			require.True(t, leaf.IPAddresses[0].Equal(wantIP))
			require.Equal(t, tc.sanHost, leaf.Subject.CommonName)
			require.ElementsMatch(t, []x509.ExtKeyUsage{
				x509.ExtKeyUsageServerAuth,
				x509.ExtKeyUsageClientAuth,
			}, leaf.ExtKeyUsage)

			for _, usage := range leaf.ExtKeyUsage {
				_, err = leaf.Verify(x509.VerifyOptions{
					Roots:     cfg.RootCAs,
					KeyUsages: []x509.ExtKeyUsage{usage},
				})
				require.NoError(t, err)
			}
			require.NoError(t, leaf.VerifyHostname(tc.sanHost))

			now := time.Now()
			require.True(t, leaf.NotBefore.Before(now.Add(time.Minute)))
			require.WithinDuration(t, now.Add(leafCertValidity), leaf.NotAfter, time.Minute)
		})
	}
}

func Test_buildClusterTLSConfig_UniqueLeafKeys(t *testing.T) {
	t.Parallel()

	caCert, caKey := generateTestCA(t)
	opts := ClusterTLSOptions{
		CACert:  caCert,
		CAKey:   caKey,
		SANHost: "127.0.0.1",
	}

	first, err := buildClusterTLSConfig(opts)
	require.NoError(t, err)
	second, err := buildClusterTLSConfig(opts)
	require.NoError(t, err)

	// Each replica mints its own ephemeral key; two builds must never
	// share key material or serial numbers.
	firstKey, ok := first.Certificates[0].PrivateKey.(*ecdsa.PrivateKey)
	require.True(t, ok)
	secondKey, ok := second.Certificates[0].PrivateKey.(*ecdsa.PrivateKey)
	require.True(t, ok)
	require.False(t, firstKey.Equal(secondKey))
	require.NotEqual(t, first.Certificates[0].Leaf.SerialNumber, second.Certificates[0].Leaf.SerialNumber)
}

func Test_buildServerOptions_ClusterTLS(t *testing.T) {
	t.Parallel()

	caCert, caKey := generateTestCA(t)

	t.Run("Enabled", func(t *testing.T) {
		t.Parallel()

		sopts, err := buildServerOptions(Options{
			ClusterAuthToken: "token",
			ClusterTLS: &ClusterTLSOptions{
				CACert:  caCert,
				CAKey:   caKey,
				SANHost: "127.0.0.1",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, sopts.Cluster.TLSConfig)
		require.Equal(t, tls.RequireAndVerifyClientCert, sopts.Cluster.TLSConfig.ClientAuth)
		require.Equal(t, clusterTLSTimeout.Seconds(), sopts.Cluster.TLSTimeout)
	})

	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()

		sopts, err := buildServerOptions(Options{
			ClusterAuthToken: "token",
		})
		require.NoError(t, err)
		require.Nil(t, sopts.Cluster.TLSConfig)
		require.Zero(t, sopts.Cluster.TLSTimeout)
	})

	t.Run("InvalidOptions", func(t *testing.T) {
		t.Parallel()

		_, err := buildServerOptions(Options{
			ClusterTLS: &ClusterTLSOptions{
				CACert:  caCert,
				CAKey:   caKey,
				SANHost: "not-an-ip",
			},
		})
		require.ErrorContains(t, err, "is not an IP address")
	})
}

func Test_ClusterTLSOptionsFromRelayURL(t *testing.T) {
	t.Parallel()

	caCert, caKey := generateTestCA(t)

	mustParse := func(raw string) *url.URL {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		return u
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		opts, err := ClusterTLSOptionsFromRelayURL(mustParse("http://10.0.1.5:3457"), caCert, caKey)
		require.NoError(t, err)
		require.Equal(t, "10.0.1.5", opts.SANHost)
		require.Equal(t, caCert, opts.CACert)
	})

	t.Run("IPv6", func(t *testing.T) {
		t.Parallel()

		opts, err := ClusterTLSOptionsFromRelayURL(mustParse("http://[fd12:3456:789a::1]:3457"), caCert, caKey)
		require.NoError(t, err)
		require.Equal(t, "fd12:3456:789a::1", opts.SANHost)
	})

	t.Run("NilRelayURL", func(t *testing.T) {
		t.Parallel()

		_, err := ClusterTLSOptionsFromRelayURL(nil, caCert, caKey)
		require.ErrorContains(t, err, "relay URL is required")
	})

	t.Run("HostnameRelayURL", func(t *testing.T) {
		t.Parallel()

		_, err := ClusterTLSOptionsFromRelayURL(mustParse("http://coderd-0.coderd:3457"), caCert, caKey)
		require.ErrorContains(t, err, "is not an IP address")
	})

	t.Run("MissingCA", func(t *testing.T) {
		t.Parallel()

		_, err := ClusterTLSOptionsFromRelayURL(mustParse("http://10.0.1.5:3457"), nil, nil)
		require.ErrorContains(t, err, "CA certificate is required")
	})
}
