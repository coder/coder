package oauth2dcr_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/oauth2dcr"
	"github.com/coder/coder/v2/testutil"
)

func TestGetOAuth2AuthorizationServerMetadata(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name            string
			issuerName      string
			expectIssuerURL func(t *testing.T, issuerURL string)
		}{
			{
				name:       "NoPath",
				issuerName: "",
				expectIssuerURL: func(t *testing.T, issuerURL string) {
					u, err := url.Parse(issuerURL)
					require.NoError(t, err)
					require.Empty(t, u.Path)
				},
			},
			{
				name:       "WithPath",
				issuerName: "test",
				expectIssuerURL: func(t *testing.T, issuerURL string) {
					u, err := url.Parse(issuerURL)
					require.NoError(t, err)
					require.Equal(t, "/test", u.Path)
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitLong)

				issuer := fakeIssuerServer(t, fakeIssuerServerOpts{
					issuerName: tc.issuerName,
					metadataEndpointHandler: func(t *testing.T, issuer fakeIssuer) http.HandlerFunc {
						return func(rw http.ResponseWriter, r *http.Request) {
							writeJSON(t, rw, map[string]any{
								"issuer":                issuer.issuerURL,
								"registration_endpoint": issuer.registrationEndpointURL,
							})
						}
					},
				})
				tc.expectIssuerURL(t, issuer.issuerURL)

				client := insecureClient(t)
				res, err := oauth2dcr.GetOAuth2AuthorizationServerMetadata(ctx, client, issuer.issuerURL)
				require.NoError(t, err)
				require.Equal(t, oauth2dcr.OAuth2AuthorizationServerMetadataResponse{
					Issuer:               issuer.issuerURL,
					RegistrationEndpoint: issuer.registrationEndpointURL,
				}, res)
			})
		}
	})

	t.Run("InvalidIssuerURL", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		// not a valid URL
		client := insecureClient(t)
		_, err := oauth2dcr.GetOAuth2AuthorizationServerMetadata(ctx, client, string([]byte{0x7f}))
		require.ErrorContains(t, err, "invalid server URL")

		// http is not supported
		_, err = oauth2dcr.GetOAuth2AuthorizationServerMetadata(ctx, client, "http://127.0.0.1:1")
		require.ErrorContains(t, err, "server URL must be HTTPS")
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		issuer := fakeIssuerServer(t, fakeIssuerServerOpts{
			metadataEndpointHandler: func(t *testing.T, issuer fakeIssuer) http.HandlerFunc {
				return func(rw http.ResponseWriter, r *http.Request) {
					rw.WriteHeader(http.StatusNotFound)
				}
			},
		})

		client := insecureClient(t)
		_, err := oauth2dcr.GetOAuth2AuthorizationServerMetadata(ctx, client, issuer.issuerURL)
		require.ErrorIs(t, err, oauth2dcr.ErrMetadataEndpointNotFound)
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		issuer := fakeIssuerServer(t, fakeIssuerServerOpts{
			metadataEndpointHandler: func(t *testing.T, issuer fakeIssuer) http.HandlerFunc {
				return func(rw http.ResponseWriter, r *http.Request) {
					rw.WriteHeader(http.StatusInternalServerError)
				}
			},
		})

		client := insecureClient(t)
		_, err := oauth2dcr.GetOAuth2AuthorizationServerMetadata(ctx, client, issuer.issuerURL)
		require.Error(t, err)
		require.NotErrorIs(t, err, oauth2dcr.ErrMetadataEndpointNotFound)
	})

	t.Run("NotJSON", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		issuer := fakeIssuerServer(t, fakeIssuerServerOpts{
			metadataEndpointHandler: func(t *testing.T, issuer fakeIssuer) http.HandlerFunc {
				return func(rw http.ResponseWriter, r *http.Request) {
					rw.WriteHeader(http.StatusOK)
					rw.Write([]byte("not JSON"))
				}
			},
		})

		client := insecureClient(t)

		_, err := oauth2dcr.GetOAuth2AuthorizationServerMetadata(ctx, client, issuer.issuerURL)
		require.ErrorContains(t, err, "could not decode JSON response")
	})

	t.Run("IssuerURL/Mismatch", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		issuer := fakeIssuerServer(t, fakeIssuerServerOpts{
			metadataEndpointHandler: func(t *testing.T, issuer fakeIssuer) http.HandlerFunc {
				return func(rw http.ResponseWriter, r *http.Request) {
					writeJSON(t, rw, map[string]any{
						"issuer":                "different-issuer-url",
						"registration_endpoint": oauth2RegistrationEndpoint,
					})
				}
			},
		})

		client := insecureClient(t)
		_, err := oauth2dcr.GetOAuth2AuthorizationServerMetadata(ctx, client, issuer.issuerURL)
		require.ErrorContains(t, err, "metadata endpoint returned issuer URL that does not match provided issuer URL")
	})
}
