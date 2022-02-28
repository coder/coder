package codersdk_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"cloud.google.com/go/compute/metadata"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
)

func TestAuthenticateWorkspaceAgentUsingGoogleCloudIdentity(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.AuthenticateWorkspaceAgentUsingGoogleCloudIdentity(context.Background(), "", metadata.NewClient(&http.Client{
			Transport: roundTripper(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte("sometoken"))),
				}, nil
			}),
		}))
		require.Error(t, err)
	})
}

type roundTripper func(req *http.Request) (*http.Response, error)

func (r roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}
