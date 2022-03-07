package coderd_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"net/http"
	"testing"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestPostWorkspaceAuthGoogleInstanceIdentity(t *testing.T) {
	t.Parallel()
	t.Run("Expired", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		signedKey, keyID, privateKey := createSignedToken(t, instanceID, &jwt.MapClaims{})
		validator := createValidator(t, keyID, privateKey)
		client := coderdtest.New(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		})
		_, err := client.AuthWorkspaceGoogleInstanceIdentity(context.Background(), "", createMetadataClient(signedKey))
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})

	t.Run("InstanceNotFound", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		signedKey, keyID, privateKey := createSignedToken(t, instanceID, nil)
		validator := createValidator(t, keyID, privateKey)
		client := coderdtest.New(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		})
		_, err := client.AuthWorkspaceGoogleInstanceIdentity(context.Background(), "", createMetadataClient(signedKey))
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		instanceID := "instanceidentifier"
		signedKey, keyID, privateKey := createSignedToken(t, instanceID, nil)
		validator := createValidator(t, keyID, privateKey)
		client := coderdtest.New(t, &coderdtest.Options{
			GoogleTokenValidator: validator,
		})
		user := coderdtest.CreateFirstUser(t, client)
		coderdtest.NewProvisionerDaemon(t, client)
		version := coderdtest.CreateProjectVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse: echo.ParseComplete,
			Provision: []*proto.Provision_Response{{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Resources: []*proto.Resource{{
							Name: "somename",
							Type: "someinstance",
							Agent: &proto.Agent{
								Auth: &proto.Agent_GoogleInstanceIdentity{
									GoogleInstanceIdentity: &proto.GoogleInstanceIdentityAuth{
										InstanceId: instanceID,
									},
								},
							},
						}},
					},
				},
			}},
		})
		project := coderdtest.CreateProject(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitProjectVersionJob(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, "me", project.ID)
		build, err := client.CreateWorkspaceBuild(context.Background(), workspace.ID, coderd.CreateWorkspaceBuildRequest{
			ProjectVersionID: project.ActiveVersionID,
			Transition:       database.WorkspaceTransitionStart,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		_, err = client.AuthWorkspaceGoogleInstanceIdentity(context.Background(), "", createMetadataClient(signedKey))
		require.NoError(t, err)
	})
}

// Used to easily create an HTTP transport!
type roundTripper func(req *http.Request) (*http.Response, error)

func (r roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

// Create's a new Google metadata client to authenticate.
func createMetadataClient(signedKey string) *metadata.Client {
	return metadata.NewClient(&http.Client{
		Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(bytes.NewReader([]byte(signedKey))),
				Header:     make(http.Header),
			}, nil
		}),
	})
}

// Create's a signed JWT with a randomly generated private key.
func createSignedToken(t *testing.T, instanceID string, claims *jwt.MapClaims) (signedKey string, keyID string, privateKey *rsa.PrivateKey) {
	keyID, err := cryptorand.String(12)
	require.NoError(t, err)
	if claims == nil {
		claims = &jwt.MapClaims{
			"exp": time.Now().AddDate(1, 0, 0).Unix(),
			"google": map[string]interface{}{
				"compute_engine": map[string]string{
					"instance_id": instanceID,
				},
			},
		}
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	privateKey, err = rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signedKey, err = token.SignedString(privateKey)
	require.NoError(t, err)
	return signedKey, keyID, privateKey
}

// Create's a validator that verifies against the provided private key.
// In a production scenario, the validator calls against the Google OAuth API
// to obtain certificates.
func createValidator(t *testing.T, keyID string, privateKey *rsa.PrivateKey) *idtoken.Validator {
	// Taken from: https://github.com/googleapis/google-api-go-client/blob/4bb729045d611fa77bdbeb971f6a1204ba23161d/idtoken/validate.go#L57-L75
	type jwk struct {
		Kid string `json:"kid"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	type certResponse struct {
		Keys []jwk `json:"keys"`
	}

	validator, err := idtoken.NewValidator(context.Background(), option.WithHTTPClient(&http.Client{
		Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
			data, err := json.Marshal(certResponse{
				Keys: []jwk{{
					Kid: keyID,
					N:   base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes()),
					E:   base64.RawURLEncoding.EncodeToString(new(big.Int).SetInt64(int64(privateKey.E)).Bytes()),
				}},
			})
			require.NoError(t, err)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(bytes.NewReader(data)),
				Header:     make(http.Header),
			}, nil
		}),
	}))
	require.NoError(t, err)
	return validator
}
