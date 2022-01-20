//go:build linux

package terraform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/require"
	"storj.io/drpc/drpcconn"

	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"

	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
)

func TestProvision(t *testing.T) {
	t.Parallel()

	installer := &releases.ExactVersion{
		Product: product.Terraform,
		Version: version.Must(version.NewVersion("1.1.2")),
	}
	execPath, err := installer.Install(context.Background())
	require.NoError(t, err)

	client, server := provisionersdk.TransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		cancelFunc()
	})
	go func() {
		err := Serve(ctx, &ServeOptions{
			ServeOptions: &provisionersdk.ServeOptions{
				Transport: server,
			},
			BinaryPath: execPath,
		})
		require.NoError(t, err)
	}()
	api := proto.NewDRPCProvisionerClient(drpcconn.New(client))

	for _, testCase := range []struct {
		Name     string
		Files    map[string]string
		Request  *proto.Provision_Request
		Response *proto.Provision_Response
		Error    bool
	}{{
		Name: "single-variable",
		Files: map[string]string{
			"main.tf": `variable "A" {
				description = "Testing!"
			}`,
		},
		Request: &proto.Provision_Request{
			ParameterValues: []*proto.ParameterValue{{
				Name:  "A",
				Value: "example",
			}},
		},
		Response: &proto.Provision_Response{},
	}, {
		Name: "missing-variable",
		Files: map[string]string{
			"main.tf": `variable "A" {
			}`,
		},
		Error: true,
	}, {
		Name: "single-resource",
		Files: map[string]string{
			"main.tf": `resource "null_resource" "A" {}`,
		},
		Response: &proto.Provision_Response{
			Resources: []*proto.Resource{{
				Name: "A",
				Type: "null_resource",
			}},
		},
	}, {
		Name: "invalid-sourcecode",
		Files: map[string]string{
			"main.tf": `a`,
		},
		Error: true,
	}} {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()

			directory := t.TempDir()
			for path, content := range testCase.Files {
				err := os.WriteFile(filepath.Join(directory, path), []byte(content), 0600)
				require.NoError(t, err)
			}

			request := &proto.Provision_Request{
				Directory: directory,
			}
			if testCase.Request != nil {
				request.ParameterValues = testCase.Request.ParameterValues
				request.State = testCase.Request.State
			}
			response, err := api.Provision(ctx, request)
			if testCase.Error {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Greater(t, len(response.State), 0)

			resourcesGot, err := json.Marshal(response.Resources)
			require.NoError(t, err)

			resourcesWant, err := json.Marshal(testCase.Response.Resources)
			require.NoError(t, err)

			require.Equal(t, string(resourcesWant), string(resourcesGot))
		})
	}
}
