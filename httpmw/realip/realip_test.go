package realip_test

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/httpmw/realip"
)

// TestExtractAddress checks the ExtractAddress function.
func TestExtractAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name               string
		Config             *realip.Config
		Header             http.Header
		RemoteAddr         string
		TLS                bool
		ExpectedRemoteAddr string
		ExpectedTLS        bool
	}{
		{
			Name:               "default-nil-config",
			RemoteAddr:         "123.45.67.89",
			ExpectedRemoteAddr: "123.45.67.89",
		},
		{
			Name:               "default-empty-config",
			RemoteAddr:         "123.45.67.89",
			ExpectedRemoteAddr: "123.45.67.89",
			Config:             &realip.Config{},
		},
		{
			Name: "default-filter-headers",
			Config: &realip.Config{
				TrustedOrigins: []*net.IPNet{
					{
						IP:   net.ParseIP("10.0.0.0"),
						Mask: net.CIDRMask(8, 32),
					},
				},
			},
			RemoteAddr: "123.45.67.89",
			Header: http.Header{
				"X-Forwarded-For": []string{
					"127.0.0.1",
					"10.0.0.5",
					"10.0.0.5,4.4.4.4",
				},
			},
			ExpectedRemoteAddr: "123.45.67.89",
		},
		{
			Name: "multiple-x-forwarded-for",
			Config: &realip.Config{
				TrustedOrigins: []*net.IPNet{
					{
						IP:   net.ParseIP("0.0.0.0"),
						Mask: net.CIDRMask(0, 32),
					},
				},
				XForwardedFor: true,
			},
			RemoteAddr: "123.45.67.89",
			Header: http.Header{
				"X-Forwarded-For": []string{
					"10.24.1.1,1.2.3.4,1.1.1.1,4.5.6.7",
					"10.0.0.5",
					"10.0.0.5,4.4.4.4",
				},
			},
			ExpectedRemoteAddr: "10.24.1.1",
		},
		{
			Name: "single-real-ip",
			Config: &realip.Config{
				TrustedOrigins: []*net.IPNet{
					{
						IP:   net.ParseIP("0.0.0.0"),
						Mask: net.CIDRMask(0, 32),
					},
				},
				XRealIP: true,
			},
			RemoteAddr: "123.45.67.89",
			TLS:        true,
			Header: http.Header{
				"X-Real-Ip": []string{"8.8.8.8"},
			},
			ExpectedRemoteAddr: "8.8.8.8",
			ExpectedTLS:        true,
		},
		{
			Name: "multiple-real-ip",
			Config: &realip.Config{
				TrustedOrigins: []*net.IPNet{
					{
						IP:   net.ParseIP("0.0.0.0"),
						Mask: net.CIDRMask(0, 32),
					},
				},
				XRealIP: true,
			},
			RemoteAddr: "123.45.67.89",
			Header: http.Header{
				"X-Real-Ip": []string{"4.4.4.4", "8.8.8.8"},
			},
			ExpectedRemoteAddr: "4.4.4.4",
		},
		{
			// Has X-Forwarded-For and X-Real-Ip, prefers X-Real-Ip
			Name: "prefer-real-ip",
			Config: &realip.Config{
				TrustedOrigins: []*net.IPNet{
					{
						IP:   net.ParseIP("0.0.0.0"),
						Mask: net.CIDRMask(0, 32),
					},
				},
				CloudflareConnectingIP: true,
				TrueClientIP:           true,
				XRealIP:                true,
				XForwardedFor:          true,
			},
			RemoteAddr: "123.45.67.89",
			Header: http.Header{
				"X-Forwarded-For": []string{"8.8.8.8"},
				"X-Real-Ip":       []string{"4.4.4.4"},
			},
			ExpectedRemoteAddr: "4.4.4.4",
		},
		{
			// Has X-Forwarded-For, X-Real-Ip, and True-Client-Ip, prefers
			// True-Client-Ip
			Name: "prefer-true-client-ip",
			Config: &realip.Config{
				TrustedOrigins: []*net.IPNet{
					{
						IP:   net.ParseIP("123.45.0.0"),
						Mask: net.CIDRMask(16, 32),
					},
				},
				CloudflareConnectingIP: true,
				TrueClientIP:           true,
				XRealIP:                true,
				XForwardedFor:          true,
			},
			RemoteAddr: "123.45.67.89",
			TLS:        true,
			Header: http.Header{
				"X-Forwarded-For": []string{"1.2.3.4"},
				"X-Real-Ip":       []string{"4.4.4.4", "8.8.8.8"},
				"True-Client-Ip":  []string{"5.6.7.8", "9.8.7.6"},
			},
			ExpectedRemoteAddr: "5.6.7.8",
			ExpectedTLS:        true,
		},
		{
			// Has X-Forwarded-For, X-Real-Ip, True-Client-Ip, and
			// Cf-Connecting-Ip, prefers Cf-Connecting-Ip
			Name: "prefer-cf-connecting-ip",
			Config: &realip.Config{
				TrustedOrigins: []*net.IPNet{
					{
						IP:   net.ParseIP("123.45.67.89"),
						Mask: net.CIDRMask(32, 32),
					},
				},
				CloudflareConnectingIP: true,
				TrueClientIP:           true,
				XRealIP:                true,
				XForwardedFor:          true,
			},
			RemoteAddr: "123.45.67.89",
			Header: http.Header{
				"X-Forwarded-For":  []string{"1.2.3.4,100.12.1.3,10.10.10.10"},
				"X-Real-Ip":        []string{"4.4.4.4", "8.8.8.8"},
				"True-Client-Ip":   []string{"5.6.7.8", "9.8.7.6"},
				"Cf-Connecting-Ip": []string{"100.10.2.2"},
			},
			ExpectedRemoteAddr: "100.10.2.2",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)

			// Default to a direct (unproxied) connection over HTTP
			req.RemoteAddr = test.RemoteAddr
			if test.TLS {
				req.TLS = &tls.ConnectionState{}
			} else {
				req.TLS = nil
			}
			req.Header = test.Header

			info, err := realip.ExtractAddress(test.Config, req)
			require.NoError(t, err, "unexpected error in ExtractAddress")
			require.Equal(t, test.ExpectedRemoteAddr, info.IP.String(), "expected info.IP.String() to match")
			require.Equal(t, test.ExpectedTLS, info.TLS, "expected info.TLS to match")
		})
	}
}

// TestTrustedOrigins tests different settings for TrustedOrigins.
//
// Disable paralleltest as it does not work correctly with table-driven
// tests: https://github.com/kunwardeep/paralleltest/issues/8
//nolint:paralleltest
func TestTrustedOrigins(t *testing.T) {
	t.Parallel()

	// Protocol is the remote client protocol: HTTP or HTTPS
	for _, proto := range []string{"http", "https"} {
		// Trusted origin
		// all: default behavior, trust all origins
		// none: use an empty set (nothing will be accepted in this case)
		// ipv4: trust an IPv6 network
		// ipv6: trust an IPv4 network
		for _, trusted := range []string{"none", "ipv4", "ipv6"} {
			for _, header := range []string{"Cf-Connecting-Ip", "True-Client-Ip", "X-Real-Ip", "X-Forwarded-For"} {
				trusted := trusted
				proto := proto
				header := header
				name := fmt.Sprintf("%s-%s-%s", trusted, proto, strings.ToLower(header))

				t.Run(name, func(t *testing.T) {
					t.Parallel()

					remoteAddr := "10.10.10.10"
					actualAddr := "12.34.56.78"

					config := &realip.Config{
						CloudflareConnectingIP: true,
						TrueClientIP:           true,
						XRealIP:                true,
						XForwardedFor:          true,
					}
					switch trusted {
					case "none":
						config.TrustedOrigins = []*net.IPNet{}
					case "ipv4":
						config.TrustedOrigins = []*net.IPNet{
							{
								IP:   net.ParseIP("10.0.0.0"),
								Mask: net.CIDRMask(24, 32),
							},
						}
						remoteAddr = "10.0.0.1"
					case "ipv6":
						config.TrustedOrigins = []*net.IPNet{
							{
								IP:   net.ParseIP("2606:4700::0"),
								Mask: net.CIDRMask(32, 128),
							},
						}
						remoteAddr = "2606:4700:4700::1111"
					}

					middleware := realip.Middleware(config)

					req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
					req.Header.Set(header, actualAddr)
					req.RemoteAddr = remoteAddr
					if proto == "https" {
						req.TLS = &tls.ConnectionState{}
					}

					handlerCalled := false

					nextHandler := http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
						// If nothing is trusted, the remoteAddr should be unchanged
						if trusted == "none" {
							require.Equal(t, remoteAddr, req.RemoteAddr, "remote address should be unchanged")
						} else {
							require.Equal(t, actualAddr, req.RemoteAddr, "actual address should be trusted")
						}

						handlerCalled = true
					})

					middleware(nextHandler).ServeHTTP(httptest.NewRecorder(), req)

					require.True(t, handlerCalled, "expected handler to be invoked")
				})
			}
		}
	}
}

// TestCorruptedHeaders tests the middleware when the reverse proxy
// supplies unparseable content.
//
// Disable paralleltest as it does not work correctly with table-driven
// tests: https://github.com/kunwardeep/paralleltest/issues/8
//nolint:paralleltest
func TestCorruptedHeaders(t *testing.T) {
	t.Parallel()

	for _, header := range []string{"Cf-Connecting-Ip", "True-Client-Ip", "X-Real-Ip", "X-Forwarded-For"} {
		header := header
		name := strings.ToLower(header)

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			remoteAddr := "10.10.10.10"

			config := &realip.Config{
				TrustedOrigins: []*net.IPNet{
					{
						IP:   net.ParseIP("10.0.0.0"),
						Mask: net.CIDRMask(8, 32),
					},
				},
				CloudflareConnectingIP: true,
				TrueClientIP:           true,
				XRealIP:                true,
				XForwardedFor:          true,
			}

			middleware := realip.Middleware(config)

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			req.Header.Set(header, "12.34.56!78")
			req.RemoteAddr = remoteAddr

			handlerCalled := false

			nextHandler := http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
				// Since the header is unparseable, the remoteAddr should be unchanged
				require.Equal(t, remoteAddr, req.RemoteAddr, "remote address should be unchanged")

				handlerCalled = true
			})

			middleware(nextHandler).ServeHTTP(httptest.NewRecorder(), req)

			require.True(t, handlerCalled, "expected handler to be invoked")
		})
	}
}

// TestAddressFamilies tests the middleware using different combinations of
// address families for remote and proxy endpoints.
//
// Disable paralleltest as it does not work correctly with table-driven
// tests: https://github.com/kunwardeep/paralleltest/issues/8
//nolint:paralleltest
func TestAddressFamilies(t *testing.T) {
	t.Parallel()

	for _, clientFamily := range []string{"ipv4", "ipv6"} {
		for _, proxyFamily := range []string{"ipv4", "ipv6"} {
			for _, header := range []string{"Cf-Connecting-Ip", "True-Client-Ip", "X-Real-Ip", "X-Forwarded-For"} {
				clientFamily := clientFamily
				proxyFamily := proxyFamily
				header := header
				name := fmt.Sprintf("%s-%s-%s", strings.ToLower(header), clientFamily, proxyFamily)

				t.Run(name, func(t *testing.T) {
					t.Parallel()

					clientAddr := "123.123.123.123"
					if clientFamily == "ipv6" {
						clientAddr = "2a03:2880:f10c:83:face:b00c:0:25de"
					}

					proxyAddr := "4.4.4.4"
					if proxyFamily == "ipv6" {
						proxyAddr = "2001:4860:4860::8888"
					}

					config := &realip.Config{
						TrustedOrigins: []*net.IPNet{
							{
								IP:   net.ParseIP("0.0.0.0"),
								Mask: net.CIDRMask(0, 32),
							},
							{
								IP:   net.ParseIP("0::"),
								Mask: net.CIDRMask(0, 128),
							},
						},
						CloudflareConnectingIP: true,
						TrueClientIP:           true,
						XRealIP:                true,
						XForwardedFor:          true,
					}

					middleware := realip.Middleware(config)

					req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
					req.Header.Set(header, clientAddr)
					req.RemoteAddr = proxyAddr

					handlerCalled := false

					nextHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						require.Equal(t, clientAddr, req.RemoteAddr, "remote address should match remote client")

						handlerCalled = true
					})

					middleware(nextHandler).ServeHTTP(httptest.NewRecorder(), req)

					require.True(t, handlerCalled, "expected handler to be invoked")
				})
			}
		}
	}
}

// TestFilterUntrusted tests that untrusted headers are removed from the request.
func TestFilterUntrusted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name               string
		Config             *realip.Config
		Header             http.Header
		RemoteAddr         string
		ExpectedHeader     http.Header
		ExpectedRemoteAddr string
	}{
		{
			Name: "untrusted-origin",
			Config: &realip.Config{
				TrustedOrigins:         nil,
				CloudflareConnectingIP: true,
				TrueClientIP:           true,
				XRealIP:                true,
				XForwardedFor:          true,
			},
			Header: http.Header{
				"X-Forwarded-For":   []string{"1.2.3.4,123.45.67.89"},
				"X-Forwarded-Proto": []string{"https"},
				"X-Real-Ip":         []string{"4.4.4.4"},
				"True-Client-Ip":    []string{"5.6.7.8"},
				"Authorization":     []string{"Bearer 123"},
				"Accept-Encoding":   []string{"gzip", "compress", "deflate", "identity"},
			},
			RemoteAddr: "1.2.3.4",
			ExpectedHeader: http.Header{
				"Authorization":   []string{"Bearer 123"},
				"Accept-Encoding": []string{"gzip", "compress", "deflate", "identity"},
			},
			ExpectedRemoteAddr: "1.2.3.4",
		},
		{
			Name: "untrusted-header",
			Config: &realip.Config{
				TrustedOrigins: []*net.IPNet{
					{
						IP:   net.ParseIP("0.0.0.0"),
						Mask: net.CIDRMask(0, 32),
					},
				},
				XRealIP: true,
			},
			Header: http.Header{
				"X-Forwarded-For":   []string{"1.2.3.4,123.45.67.89"},
				"X-Forwarded-Proto": []string{"https"},
				"X-Real-Ip":         []string{"4.4.4.4"},
				"True-Client-Ip":    []string{"5.6.7.8"},
				"Authorization":     []string{"Bearer 123"},
				"Accept-Encoding":   []string{"gzip", "compress", "deflate", "identity"},
			},
			RemoteAddr: "1.2.3.4",
			ExpectedHeader: http.Header{
				"Authorization":   []string{"Bearer 123"},
				"Accept-Encoding": []string{"gzip", "compress", "deflate", "identity"},
				"X-Real-Ip":       []string{"4.4.4.4"},
			},
			ExpectedRemoteAddr: "1.2.3.4",
		},
		{
			Name: "keep-first-header",
			Config: &realip.Config{
				TrustedOrigins: []*net.IPNet{
					{
						IP:   net.ParseIP("0.0.0.0"),
						Mask: net.CIDRMask(0, 32),
					},
				},
				XForwardedFor: true,
			},
			Header: http.Header{
				"X-Forwarded-For":   []string{"1.2.3.4,123.45.67.89", "::1", "2001:4860:4860::8844"},
				"X-Forwarded-Proto": []string{"https"},
				"X-Real-Ip":         []string{"4.4.4.4"},
				"True-Client-Ip":    []string{"5.6.7.8"},
				"Authorization":     []string{"Bearer 123"},
				"Accept-Encoding":   []string{"gzip", "compress", "deflate", "identity"},
			},
			RemoteAddr: "1.2.3.4",
			ExpectedHeader: http.Header{
				"Authorization":     []string{"Bearer 123"},
				"Accept-Encoding":   []string{"gzip", "compress", "deflate", "identity"},
				"X-Forwarded-For":   []string{"1.2.3.4,123.45.67.89"},
				"X-Forwarded-Proto": []string{"https"},
			},
			ExpectedRemoteAddr: "1.2.3.4",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			req.Header = test.Header
			req.RemoteAddr = test.RemoteAddr

			err := realip.FilterUntrustedHeaders(test.Config, req)
			require.NoError(t, err, "filter untrusted headers should be successful")
			require.Equal(t, test.ExpectedRemoteAddr, req.RemoteAddr, "remote address should match")
			require.Equal(t, test.ExpectedHeader, req.Header, "filtered headers should match")
		})
	}
}

// TestDevURLProxy checks headers passed to DevURL services are as expected.
func TestDevURLProxy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name               string
		Config             *realip.Config
		Header             http.Header
		RemoteAddr         string
		TLS                bool
		ExpectedHeader     http.Header
		ExpectedTLS        bool
		ExpectedRemoteAddr string
	}{
		{
			Name:   "untrusted-origin-http",
			Config: nil,
			Header: http.Header{
				"X-Forwarded-For": []string{"123.45.67.89,10.10.10.10"},
			},
			RemoteAddr: "17.18.19.20",
			TLS:        false,
			ExpectedHeader: http.Header{
				"X-Forwarded-For":   []string{"17.18.19.20"},
				"X-Forwarded-Proto": []string{"http"},
			},
			ExpectedTLS:        false,
			ExpectedRemoteAddr: "17.18.19.20",
		},
		{
			Name:   "untrusted-origin-https",
			Config: nil,
			Header: http.Header{
				"X-Forwarded-For": []string{"123.45.67.89,10.10.10.10"},
			},
			RemoteAddr: "17.18.19.20",
			TLS:        true,
			ExpectedHeader: http.Header{
				"X-Forwarded-For":   []string{"17.18.19.20"},
				"X-Forwarded-Proto": []string{"https"},
			},
			ExpectedTLS:        true,
			ExpectedRemoteAddr: "17.18.19.20",
		},
		{
			Name: "trusted-real-ip",
			Config: &realip.Config{
				TrustedOrigins: []*net.IPNet{
					{
						IP:   net.ParseIP("0.0.0.0"),
						Mask: net.CIDRMask(0, 32),
					},
				},
				XRealIP: true,
			},
			Header: http.Header{
				"X-Real-Ip":         []string{"99.88.77.66"},
				"X-Forwarded-For":   []string{"123.45.67.89,10.10.10.10"},
				"X-Forwarded-Proto": []string{"https"},
			},
			RemoteAddr: "17.18.19.20",
			TLS:        false,
			ExpectedHeader: http.Header{
				"X-Real-Ip":         []string{"99.88.77.66"},
				"X-Forwarded-For":   []string{"99.88.77.66,17.18.19.20"},
				"X-Forwarded-Proto": []string{"https"},
			},
			// Trusted origin, accept X-Forwarded-Proto even though we don't
			// trust X-Forwarded-For
			ExpectedTLS:        true,
			ExpectedRemoteAddr: "99.88.77.66",
		},
		{
			Name: "trusted-real-ip-and-forwarded-conflict",
			Config: &realip.Config{
				TrustedOrigins: []*net.IPNet{
					{
						IP:   net.ParseIP("0.0.0.0"),
						Mask: net.CIDRMask(0, 32),
					},
				},
				XRealIP:       true,
				XForwardedFor: true,
			},
			Header: http.Header{
				"X-Real-Ip":         []string{"99.88.77.66"},
				"X-Forwarded-For":   []string{"123.45.67.89,10.10.10.10"},
				"X-Forwarded-Proto": []string{"https"},
			},
			RemoteAddr: "17.18.19.20",
			TLS:        false,
			ExpectedHeader: http.Header{
				"X-Real-Ip": []string{"99.88.77.66"},
				// Even though X-Real-Ip and X-Forwarded-For are both trusted,
				// ignore the value of X-Forwarded-For, since they conflict
				"X-Forwarded-For":   []string{"99.88.77.66,17.18.19.20"},
				"X-Forwarded-Proto": []string{"https"},
			},
			ExpectedTLS:        true,
			ExpectedRemoteAddr: "99.88.77.66",
		},
		{
			Name: "trusted-real-ip-and-forwarded-same",
			Config: &realip.Config{
				TrustedOrigins: []*net.IPNet{
					{
						IP:   net.ParseIP("0.0.0.0"),
						Mask: net.CIDRMask(0, 32),
					},
				},
				XRealIP:       true,
				XForwardedFor: true,
			},
			Header: http.Header{
				"X-Real-Ip": []string{"99.88.77.66"},
				// X-Real-Ip and X-Forwarded-For are both trusted, and since
				// they match, append the proxy address to X-Forwarded-For
				"X-Forwarded-For":   []string{"99.88.77.66,123.45.67.89,10.10.10.10"},
				"X-Forwarded-Proto": []string{"https"},
			},
			RemoteAddr: "17.18.19.20",
			TLS:        false,
			ExpectedHeader: http.Header{
				"X-Real-Ip":         []string{"99.88.77.66"},
				"X-Forwarded-For":   []string{"99.88.77.66,123.45.67.89,10.10.10.10,17.18.19.20"},
				"X-Forwarded-Proto": []string{"https"},
			},
			ExpectedTLS:        true,
			ExpectedRemoteAddr: "99.88.77.66",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			req.Header = test.Header
			req.RemoteAddr = test.RemoteAddr
			if test.TLS {
				req.TLS = &tls.ConnectionState{}
			} else {
				req.TLS = nil
			}

			middleware := realip.Middleware(test.Config)

			handlerCalled := false

			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				require.Equal(t, test.ExpectedRemoteAddr, req.RemoteAddr, "remote address should match")
				require.Equal(t, test.ExpectedTLS, req.TLS != nil, "TLS state should match")

				err := realip.FilterUntrustedHeaders(test.Config, req)
				require.NoError(t, err, "filter untrusted headers should be successful")

				err = realip.EnsureXForwardedFor(req)
				require.NoError(t, err, "ensure X-Forwarded-For should be successful")

				require.Equal(t, test.ExpectedHeader, req.Header, "filtered headers should match")

				handlerCalled = true
			})

			middleware(nextHandler).ServeHTTP(httptest.NewRecorder(), req)

			require.True(t, handlerCalled, "expected handler to be invoked")
		})
	}
}
