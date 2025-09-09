package oauth2dcr_test

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/oauth2dcr"
	"github.com/coder/coder/v2/testutil"
)

const (
	oauth2MetadataEndpoint     = "/.well-known/oauth-authorization-server"
	oauth2RegistrationEndpoint = "/client"
)

func TestRegisterDynamicClientWithMetadata(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		registrationRequest := oauth2dcr.OAuth2ClientRegistrationRequest{
			ClientName:    "test",
			RedirectURIs:  []string{"http://localhost:8080/callback"},
			GrantTypes:    []string{"authorization_code"},
			ResponseTypes: []string{"code"},
			Scope:         "cool scope",
		}

		testCases := []struct {
			name            string
			issuerName      string
			expectIssuerURL func(t *testing.T, issuerURL string)
		}{
			{
				name:       "OK",
				issuerName: "",
				expectIssuerURL: func(t *testing.T, issuerURL string) {
					u, err := url.Parse(issuerURL)
					require.NoError(t, err)
					require.Empty(t, u.Path)
				},
			},
			{
				name:       "OKWithPath",
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

				registrationResponse := oauth2dcr.OAuth2ClientRegistrationResponse{
					OAuth2ClientRegistrationRequest: registrationRequest,
					ClientID:                        "test-client-id",
					ClientSecret:                    "test-client-secret",
					ClientSecretExpiresAt:           time.Now().Add(time.Hour * 24).Unix(),
				}

				issuer := fakeIssuerServer(t, fakeIssuerServerOpts{
					issuerName: tc.issuerName,
					registrationEndpointHandler: func(rw http.ResponseWriter, r *http.Request) {
						var got oauth2dcr.OAuth2ClientRegistrationRequest
						err := json.NewDecoder(r.Body).Decode(&got)
						assert.NoError(t, err)
						assert.Equal(t, registrationRequest, got)
						writeJSON(t, rw, registrationResponse)
					},
				})
				tc.expectIssuerURL(t, issuer.issuerURL)

				metadata := oauth2dcr.OAuth2AuthorizationServerMetadataResponse{
					Issuer:               issuer.issuerURL,
					RegistrationEndpoint: issuer.registrationEndpointURL,
				}

				client := insecureClient(t)
				res, err := oauth2dcr.RegisterDynamicClientWithMetadata(ctx, client, metadata, registrationRequest)
				require.NoError(t, err)
				require.Equal(t, registrationResponse, res)
			})
		}
	})

	t.Run("Metadata/RegistrationEndpoint/Empty", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		metadata := oauth2dcr.OAuth2AuthorizationServerMetadataResponse{
			Issuer:               "does not matter",
			RegistrationEndpoint: "",
		}

		client := insecureClient(t)
		_, err := oauth2dcr.RegisterDynamicClientWithMetadata(ctx, client, metadata, oauth2dcr.OAuth2ClientRegistrationRequest{})
		require.ErrorIs(t, err, oauth2dcr.ErrDynamicClientRegistrationNotSupported)
	})

	t.Run("Metadata/RegistrationEndpoint/InvalidURL", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		metadata := oauth2dcr.OAuth2AuthorizationServerMetadataResponse{
			Issuer:               "does not matter",
			RegistrationEndpoint: string([]byte{0x7f}),
		}

		client := insecureClient(t)
		_, err := oauth2dcr.RegisterDynamicClientWithMetadata(ctx, client, metadata, oauth2dcr.OAuth2ClientRegistrationRequest{})
		require.ErrorContains(t, err, "metadata contained invalid registration endpoint")
	})

	t.Run("Metadata/RegistrationEndpoint/NotHTTPS", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		metadata := oauth2dcr.OAuth2AuthorizationServerMetadataResponse{
			Issuer:               "does not matter",
			RegistrationEndpoint: "http://localhost:8080/register",
		}

		client := insecureClient(t)
		_, err := oauth2dcr.RegisterDynamicClientWithMetadata(ctx, client, metadata, oauth2dcr.OAuth2ClientRegistrationRequest{})
		require.ErrorContains(t, err, "metadata contained registration endpoint URL that is not HTTPS")
	})

	t.Run("Metadata/ValidateRegistrationRequest", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name                        string
			mutateMetadataFn            func(metadata *oauth2dcr.OAuth2AuthorizationServerMetadataResponse)
			mutateRegistrationRequestFn func(registrationRequest *oauth2dcr.OAuth2ClientRegistrationRequest)
			expectErrorContains         string
		}{
			{
				name: "ResponseTypes/NoMetadataSpecified",
				mutateMetadataFn: func(metadata *oauth2dcr.OAuth2AuthorizationServerMetadataResponse) {
					metadata.ResponseTypesSupported = []string{}
				},
				mutateRegistrationRequestFn: func(registrationRequest *oauth2dcr.OAuth2ClientRegistrationRequest) {
					registrationRequest.ResponseTypes = []string{"code"}
				},
				expectErrorContains: "",
			},
			{
				name: "ResponseTypes/NoneSpecified",
				mutateMetadataFn: func(metadata *oauth2dcr.OAuth2AuthorizationServerMetadataResponse) {
					metadata.ResponseTypesSupported = []string{"code"}
				},
				mutateRegistrationRequestFn: func(registrationRequest *oauth2dcr.OAuth2ClientRegistrationRequest) {
					registrationRequest.ResponseTypes = []string{}
				},
				expectErrorContains: "",
			},
			{
				name: "ResponseTypes/Mismatch",
				mutateMetadataFn: func(metadata *oauth2dcr.OAuth2AuthorizationServerMetadataResponse) {
					metadata.ResponseTypesSupported = []string{"code"}
				},
				mutateRegistrationRequestFn: func(registrationRequest *oauth2dcr.OAuth2ClientRegistrationRequest) {
					registrationRequest.ResponseTypes = []string{"token"}
				},
				expectErrorContains: "metadata claims that server does not support response type",
			},
			{
				name: "Scopes/NoMetadataSpecified",
				mutateMetadataFn: func(metadata *oauth2dcr.OAuth2AuthorizationServerMetadataResponse) {
					metadata.ScopesSupported = []string{}
				},
				mutateRegistrationRequestFn: func(registrationRequest *oauth2dcr.OAuth2ClientRegistrationRequest) {
					registrationRequest.Scope = "cool scope"
				},
				expectErrorContains: "",
			},
			{
				name: "Scopes/NoneSpecified",
				mutateMetadataFn: func(metadata *oauth2dcr.OAuth2AuthorizationServerMetadataResponse) {
					metadata.ScopesSupported = []string{"cool scope"}
				},
				mutateRegistrationRequestFn: func(registrationRequest *oauth2dcr.OAuth2ClientRegistrationRequest) {
					registrationRequest.Scope = ""
				},
				expectErrorContains: "",
			},
			{
				name: "Scopes/Mismatch",
				mutateMetadataFn: func(metadata *oauth2dcr.OAuth2AuthorizationServerMetadataResponse) {
					metadata.ScopesSupported = []string{"cool_scope"}
				},
				mutateRegistrationRequestFn: func(registrationRequest *oauth2dcr.OAuth2ClientRegistrationRequest) {
					registrationRequest.Scope = "cool_scope uncool_scope"
				},
				expectErrorContains: `metadata claims that server does not support scope "uncool_scope"`,
			},
			{
				name: "GrantTypes/NoMetadataSpecified",
				mutateMetadataFn: func(metadata *oauth2dcr.OAuth2AuthorizationServerMetadataResponse) {
					metadata.GrantTypesSupported = []string{}
				},
				mutateRegistrationRequestFn: func(registrationRequest *oauth2dcr.OAuth2ClientRegistrationRequest) {
					registrationRequest.Scope = "cool_scope"
				},
				expectErrorContains: "",
			},
			{
				name: "GrantTypes/NoneSpecified",
				mutateMetadataFn: func(metadata *oauth2dcr.OAuth2AuthorizationServerMetadataResponse) {
					metadata.GrantTypesSupported = []string{"authorization_code"}
				},
				mutateRegistrationRequestFn: func(registrationRequest *oauth2dcr.OAuth2ClientRegistrationRequest) {
					registrationRequest.GrantTypes = []string{}
				},
				expectErrorContains: "",
			},
			{
				name: "GrantTypes/Mismatch",
				mutateMetadataFn: func(metadata *oauth2dcr.OAuth2AuthorizationServerMetadataResponse) {
					metadata.GrantTypesSupported = []string{"authorization_code"}
				},
				mutateRegistrationRequestFn: func(registrationRequest *oauth2dcr.OAuth2ClientRegistrationRequest) {
					registrationRequest.GrantTypes = []string{"client_credentials"}
				},
				expectErrorContains: "metadata claims that server does not support grant type",
			},
			{
				name: "TokenEndpointAuthMethods/NoMetadataSpecified",
				mutateMetadataFn: func(metadata *oauth2dcr.OAuth2AuthorizationServerMetadataResponse) {
					metadata.TokenEndpointAuthMethodsSupported = []string{}
				},
				mutateRegistrationRequestFn: func(registrationRequest *oauth2dcr.OAuth2ClientRegistrationRequest) {
					registrationRequest.TokenEndpointAuthMethod = "client_secret_post"
				},
				expectErrorContains: "",
			},
			{
				name: "TokenEndpointAuthMethods/NoneSpecified",
				mutateMetadataFn: func(metadata *oauth2dcr.OAuth2AuthorizationServerMetadataResponse) {
					metadata.TokenEndpointAuthMethodsSupported = []string{"client_secret_basic"}
				},
				mutateRegistrationRequestFn: func(registrationRequest *oauth2dcr.OAuth2ClientRegistrationRequest) {
					registrationRequest.TokenEndpointAuthMethod = ""
				},
				expectErrorContains: "",
			},
			{
				name: "TokenEndpointAuthMethods/Mismatch",
				mutateMetadataFn: func(metadata *oauth2dcr.OAuth2AuthorizationServerMetadataResponse) {
					metadata.TokenEndpointAuthMethodsSupported = []string{"client_secret_basic"}
				},
				mutateRegistrationRequestFn: func(registrationRequest *oauth2dcr.OAuth2ClientRegistrationRequest) {
					registrationRequest.TokenEndpointAuthMethod = "client_secret_post"
				},
				expectErrorContains: "metadata claims that server does not support token endpoint auth method",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitLong)

				issuer := fakeIssuerServer(t, fakeIssuerServerOpts{
					registrationEndpointHandler: func(rw http.ResponseWriter, r *http.Request) {
						writeJSON(t, rw, map[string]any{
							"client_id":                "test-client-id",
							"client_secret":            "test-client-secret",
							"client_secret_expires_at": time.Now().Add(time.Hour * 24).Unix(),
						})
					},
				})

				metadata := oauth2dcr.OAuth2AuthorizationServerMetadataResponse{
					Issuer:               issuer.issuerURL,
					RegistrationEndpoint: issuer.registrationEndpointURL,
				}
				tc.mutateMetadataFn(&metadata)

				registrationRequest := oauth2dcr.OAuth2ClientRegistrationRequest{}
				tc.mutateRegistrationRequestFn(&registrationRequest)

				client := insecureClient(t)
				_, err := oauth2dcr.RegisterDynamicClientWithMetadata(ctx, client, metadata, registrationRequest)
				if tc.expectErrorContains != "" {
					require.ErrorContains(t, err, tc.expectErrorContains)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("RegistrationEndpoint/NotPublic", func(t *testing.T) {
		t.Parallel()

		for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
			t.Run(fmt.Sprintf("Code%d", code), func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitLong)

				issuer := fakeIssuerServer(t, fakeIssuerServerOpts{
					registrationEndpointHandler: func(rw http.ResponseWriter, r *http.Request) {
						rw.WriteHeader(code)
					},
				})

				metadata := oauth2dcr.OAuth2AuthorizationServerMetadataResponse{
					Issuer:               issuer.issuerURL,
					RegistrationEndpoint: issuer.registrationEndpointURL,
				}

				client := insecureClient(t)
				_, err := oauth2dcr.RegisterDynamicClientWithMetadata(ctx, client, metadata, oauth2dcr.OAuth2ClientRegistrationRequest{})
				require.ErrorIs(t, err, oauth2dcr.ErrDynamicClientRegistrationNotSupported)
			})
		}
	})

	t.Run("RegistrationEndpoint/OtherError", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		issuer := fakeIssuerServer(t, fakeIssuerServerOpts{
			registrationEndpointHandler: func(rw http.ResponseWriter, r *http.Request) {
				rw.WriteHeader(http.StatusInternalServerError)
			},
		})

		metadata := oauth2dcr.OAuth2AuthorizationServerMetadataResponse{
			Issuer:               issuer.issuerURL,
			RegistrationEndpoint: issuer.registrationEndpointURL,
		}

		client := insecureClient(t)
		_, err := oauth2dcr.RegisterDynamicClientWithMetadata(ctx, client, metadata, oauth2dcr.OAuth2ClientRegistrationRequest{})
		require.NotErrorIs(t, err, oauth2dcr.ErrDynamicClientRegistrationNotSupported)
		require.ErrorContains(t, err, "register client")
	})

	t.Run("RegistrationEndpoint/NotJSON", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		issuer := fakeIssuerServer(t, fakeIssuerServerOpts{
			registrationEndpointHandler: func(rw http.ResponseWriter, r *http.Request) {
				rw.WriteHeader(http.StatusOK)
				rw.Write([]byte("not JSON"))
			},
		})

		metadata := oauth2dcr.OAuth2AuthorizationServerMetadataResponse{
			Issuer:               issuer.issuerURL,
			RegistrationEndpoint: issuer.registrationEndpointURL,
		}

		client := insecureClient(t)
		_, err := oauth2dcr.RegisterDynamicClientWithMetadata(ctx, client, metadata, oauth2dcr.OAuth2ClientRegistrationRequest{})
		require.ErrorContains(t, err, "could not decode JSON response")
	})

	t.Run("RegistrationEndpoint/NoClientID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		issuer := fakeIssuerServer(t, fakeIssuerServerOpts{
			registrationEndpointHandler: func(rw http.ResponseWriter, r *http.Request) {
				writeJSON(t, rw, map[string]any{})
			},
		})

		metadata := oauth2dcr.OAuth2AuthorizationServerMetadataResponse{
			Issuer:               issuer.issuerURL,
			RegistrationEndpoint: issuer.registrationEndpointURL,
		}

		client := insecureClient(t)
		_, err := oauth2dcr.RegisterDynamicClientWithMetadata(ctx, client, metadata, oauth2dcr.OAuth2ClientRegistrationRequest{})
		require.ErrorContains(t, err, "registration endpoint did not return a client ID")
	})
}

// TestRegisterDynamicClientWithIssuer is quite simple because it just calls two
// other functions that are already thoroughly tested.
func TestRegisterDynamicClientWithIssuer(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	registrationRequest := oauth2dcr.OAuth2ClientRegistrationRequest{
		ClientName:    "test",
		RedirectURIs:  []string{"http://localhost:8080/callback"},
		GrantTypes:    []string{"authorization_code"},
		ResponseTypes: []string{"code"},
		Scope:         "cool scope",
	}

	registrationResponse := oauth2dcr.OAuth2ClientRegistrationResponse{
		OAuth2ClientRegistrationRequest: registrationRequest,
		ClientID:                        "test-client-id",
		ClientSecret:                    "test-client-secret",
		ClientSecretExpiresAt:           time.Now().Add(time.Hour * 24).Unix(),
	}

	issuer := fakeIssuerServer(t, fakeIssuerServerOpts{
		metadataEndpointHandler: okMetadataEndpointHandler,
		registrationEndpointHandler: func(rw http.ResponseWriter, r *http.Request) {
			var got oauth2dcr.OAuth2ClientRegistrationRequest
			err := json.NewDecoder(r.Body).Decode(&got)
			assert.NoError(t, err)
			assert.Equal(t, registrationRequest, got)
			writeJSON(t, rw, registrationResponse)
		},
	})

	client := insecureClient(t)
	res, err := oauth2dcr.RegisterDynamicClientWithIssuer(ctx, client, issuer.issuerURL, registrationRequest)
	require.NoError(t, err)
	require.Equal(t, registrationResponse, res)
}

func insecureClient(t *testing.T) *http.Client {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // test TLS server
			},
		},
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

type fakeIssuerServerOpts struct {
	issuerName string // appended to the issuer URL

	// If unset, the metadata endpoint will return a 404 and fail the test.
	metadataEndpointHandler func(t *testing.T, issuer fakeIssuer) http.HandlerFunc
	// If unset, the registration endpoint will return a 404 and fail the test.
	registrationEndpointHandler http.HandlerFunc
}

type fakeIssuer struct {
	serverURL               string
	issuerURL               string
	metadataEndpointURL     string
	registrationEndpointURL string
}

// fakeIssuerServer creates a fake OAuth2 issuer server that can be used for
// testing. The returned string is the issuer URL.
func fakeIssuerServer(t *testing.T, opts fakeIssuerServerOpts) fakeIssuer {
	mux := http.NewServeMux()
	srv := httptest.NewUnstartedServer(mux)
	srv.StartTLS()

	serverURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	issuerURL := *serverURL
	if opts.issuerName != "" {
		issuerURL.Path = "/" + opts.issuerName
	}

	metadataEndpointPath := oauth2MetadataEndpoint
	if opts.issuerName != "" {
		metadataEndpointPath = path.Join(oauth2MetadataEndpoint, opts.issuerName)
	}
	metadataEndpointURL := *serverURL
	metadataEndpointURL.Path = metadataEndpointPath

	registrationEndpointURL := *serverURL
	registrationEndpointURL.Path = oauth2RegistrationEndpoint

	issuer := fakeIssuer{
		serverURL:               serverURL.String(),
		issuerURL:               issuerURL.String(),
		metadataEndpointURL:     metadataEndpointURL.String(),
		registrationEndpointURL: registrationEndpointURL.String(),
	}

	metadataEndpointHandler := func(rw http.ResponseWriter, r *http.Request) {
		t.Error("metadata endpoint was called when unset")
		rw.WriteHeader(http.StatusNotFound)
		_, _ = rw.Write([]byte("not found"))
	}
	if opts.metadataEndpointHandler != nil {
		metadataEndpointHandler = opts.metadataEndpointHandler(t, issuer)
	}
	mux.HandleFunc(metadataEndpointPath, metadataEndpointHandler)

	registrationEndpointHandler := func(rw http.ResponseWriter, r *http.Request) {
		t.Error("registration endpoint was called when unset")
		rw.WriteHeader(http.StatusNotFound)
		_, _ = rw.Write([]byte("not found"))
	}
	if opts.registrationEndpointHandler != nil {
		registrationEndpointHandler = opts.registrationEndpointHandler
	}
	mux.HandleFunc(oauth2RegistrationEndpoint, registrationEndpointHandler)

	return issuer
}

func okMetadataEndpointHandler(t *testing.T, issuer fakeIssuer) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		writeJSON(t, rw, map[string]any{
			"issuer":                issuer.issuerURL,
			"registration_endpoint": issuer.registrationEndpointURL,
		})
	}
}

func writeJSON(t *testing.T, rw http.ResponseWriter, body any) {
	rw.WriteHeader(http.StatusOK)
	rw.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(rw).Encode(body)
	assert.NoError(t, err)
}
