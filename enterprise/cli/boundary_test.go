package cli_test

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	boundarycli "github.com/coder/boundary/cli"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

// Actually testing the functionality of coder/boundary takes place in the
// coder/boundary repo, since it's a dependency of coder.
// Here we want to test basically that integrating it as a subcommand doesn't break anything.
func TestBoundarySubcommand(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	inv, _ := newCLI(t, "boundary", "--help")
	pty := ptytest.New(t).Attach(inv)

	go func() {
		err := inv.WithContext(ctx).Run()
		assert.NoError(t, err)
	}()

	// Expect the --help output to include the short description.
	// We're simply confirming that `coder boundary --help` ran without a runtime error as
	// a good chunk of serpents self validation logic happens at runtime.
	pty.ExpectMatch(boundarycli.BaseCommand("dev").Short)
}

func TestBoundaryLicenseVerification(t *testing.T) {
	t.Parallel()

	t.Run("EntitledAndEnabled", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureBoundary: 1,
				},
			},
		})

		inv, conf := newCLI(t, "boundary", "--version")
		clitest.SetupConfig(t, client, conf)

		ctx := testutil.Context(t, testutil.WaitShort)
		err := inv.WithContext(ctx).Run()
		// Should succeed - boundary --version should work with valid license.
		require.NoError(t, err)
	})

	t.Run("NotEntitled", func(t *testing.T) {
		t.Parallel()

		// Create a proxy server that returns entitlements without boundary feature.
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					// No FeatureBoundary
				},
			},
		})

		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/entitlements" {
				res := codersdk.Entitlements{
					Features:         map[codersdk.FeatureName]codersdk.Feature{},
					Warnings:         []string{},
					Errors:           []string{},
					HasLicense:       true,
					Trial:            false,
					RequireTelemetry: false,
				}
				// Set boundary to not entitled, all other features to entitled.
				for _, feature := range codersdk.FeatureNames {
					if feature == codersdk.FeatureBoundary {
						// Explicitly set boundary to not entitled.
						res.Features[feature] = codersdk.Feature{
							Entitlement: codersdk.EntitlementNotEntitled,
							Enabled:     false,
						}
					} else {
						res.Features[feature] = codersdk.Feature{
							Entitlement: codersdk.EntitlementEntitled,
							Enabled:     true,
						}
					}
				}
				httpapi.Write(r.Context(), w, http.StatusOK, res)
				return
			}

			// Otherwise, proxy the request to the real API server.
			rp := httputil.NewSingleHostReverseProxy(client.URL)
			tp := &http.Transport{}
			defer tp.CloseIdleConnections()
			rp.Transport = tp
			rp.ServeHTTP(w, r)
		}))
		defer proxy.Close()

		proxyURL, err := url.Parse(proxy.URL)
		require.NoError(t, err)
		proxyClient := codersdk.New(proxyURL)
		proxyClient.SetSessionToken(client.SessionToken())
		t.Cleanup(proxyClient.HTTPClient.CloseIdleConnections)

		inv, conf := newCLI(t, "boundary", "--version")
		clitest.SetupConfig(t, proxyClient, conf)

		ctx := testutil.Context(t, testutil.WaitShort)
		err = inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.ErrorContains(t, err, "your license is not entitled to use the boundary feature")
	})

	t.Run("FeatureDisabled", func(t *testing.T) {
		t.Parallel()

		// Create a proxy server that returns entitlements with boundary disabled.
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureBoundary: 1,
				},
			},
		})

		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/entitlements" {
				res := codersdk.Entitlements{
					Features:         map[codersdk.FeatureName]codersdk.Feature{},
					Warnings:         []string{},
					Errors:           []string{},
					HasLicense:       true,
					Trial:            false,
					RequireTelemetry: false,
				}
				for _, feature := range codersdk.FeatureNames {
					if feature == codersdk.FeatureBoundary {
						// Feature is entitled but disabled.
						res.Features[feature] = codersdk.Feature{
							Entitlement: codersdk.EntitlementEntitled,
							Enabled:     false,
						}
					} else {
						res.Features[feature] = codersdk.Feature{
							Entitlement: codersdk.EntitlementEntitled,
							Enabled:     true,
						}
					}
				}
				httpapi.Write(r.Context(), w, http.StatusOK, res)
				return
			}

			// Otherwise, proxy the request to the real API server.
			rp := httputil.NewSingleHostReverseProxy(client.URL)
			tp := &http.Transport{}
			defer tp.CloseIdleConnections()
			rp.Transport = tp
			rp.ServeHTTP(w, r)
		}))
		defer proxy.Close()

		proxyURL, err := url.Parse(proxy.URL)
		require.NoError(t, err)
		proxyClient := codersdk.New(proxyURL)
		proxyClient.SetSessionToken(client.SessionToken())
		t.Cleanup(proxyClient.HTTPClient.CloseIdleConnections)

		inv, conf := newCLI(t, "boundary", "--version")
		clitest.SetupConfig(t, proxyClient, conf)

		ctx := testutil.Context(t, testutil.WaitShort)
		err = inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.ErrorContains(t, err, "the boundary feature is disabled in your deployment configuration")
	})

	t.Run("AGPLDeployment", func(t *testing.T) {
		t.Parallel()

		// Create an AGPL server (no enterprise features).
		client := coderdtest.New(t, &coderdtest.Options{})

		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/entitlements" {
				// AGPL deployments return 404 for entitlements endpoint.
				w.WriteHeader(http.StatusNotFound)
				return
			}

			// Otherwise, proxy the request to the real API server.
			rp := httputil.NewSingleHostReverseProxy(client.URL)
			tp := &http.Transport{}
			defer tp.CloseIdleConnections()
			rp.Transport = tp
			rp.ServeHTTP(w, r)
		}))
		defer proxy.Close()

		proxyURL, err := url.Parse(proxy.URL)
		require.NoError(t, err)
		proxyClient := codersdk.New(proxyURL)
		proxyClient.SetSessionToken(client.SessionToken())
		t.Cleanup(proxyClient.HTTPClient.CloseIdleConnections)

		inv, conf := newCLI(t, "boundary", "--version")
		clitest.SetupConfig(t, proxyClient, conf)

		ctx := testutil.Context(t, testutil.WaitShort)
		err = inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.ErrorContains(t, err, "your deployment appears to be an AGPL deployment")
	})

	t.Run("ChildProcessSkipsCheck", func(t *testing.T) {
		// Cannot use t.Parallel() with os.Setenv(), so this test runs sequentially.
		// When CHILD=true, the license check should be skipped.
		// This simulates boundary re-executing itself to run the target process.
		// We use a proxy that would fail the license check to verify it's skipped.
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					// No FeatureBoundary - would normally fail
				},
			},
		})

		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/entitlements" {
				// Return not entitled for boundary - this would normally cause failure.
				res := codersdk.Entitlements{
					Features:         map[codersdk.FeatureName]codersdk.Feature{},
					Warnings:         []string{},
					Errors:           []string{},
					HasLicense:       true,
					Trial:            false,
					RequireTelemetry: false,
				}
				for _, feature := range codersdk.FeatureNames {
					if feature == codersdk.FeatureBoundary {
						res.Features[feature] = codersdk.Feature{
							Entitlement: codersdk.EntitlementNotEntitled,
							Enabled:     false,
						}
					} else {
						res.Features[feature] = codersdk.Feature{
							Entitlement: codersdk.EntitlementEntitled,
							Enabled:     true,
						}
					}
				}
				httpapi.Write(r.Context(), w, http.StatusOK, res)
				return
			}

			// Otherwise, proxy the request to the real API server.
			rp := httputil.NewSingleHostReverseProxy(client.URL)
			tp := &http.Transport{}
			defer tp.CloseIdleConnections()
			rp.Transport = tp
			rp.ServeHTTP(w, r)
		}))
		defer proxy.Close()

		proxyURL, err := url.Parse(proxy.URL)
		require.NoError(t, err)
		proxyClient := codersdk.New(proxyURL)
		proxyClient.SetSessionToken(client.SessionToken())
		t.Cleanup(proxyClient.HTTPClient.CloseIdleConnections)

		inv, conf := newCLI(t, "boundary", "--version")
		clitest.SetupConfig(t, proxyClient, conf)

		// Set CHILD=true in the actual OS environment to simulate boundary re-execution.
		// This should skip the license check, so the command should succeed
		// even though the proxy would return "not entitled".
		oldValue := os.Getenv("CHILD")
		os.Setenv("CHILD", "true")
		t.Cleanup(func() {
			if oldValue == "" {
				os.Unsetenv("CHILD")
			} else {
				os.Setenv("CHILD", oldValue)
			}
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		err = inv.WithContext(ctx).Run()
		// Should succeed because license check is skipped for child processes.
		require.NoError(t, err)
	})
}
