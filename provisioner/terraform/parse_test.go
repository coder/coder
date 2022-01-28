//go:build linux

package terraform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"storj.io/drpc/drpcconn"

	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestParse(t *testing.T) {
	t.Parallel()

	// Create an in-memory provisioner to communicate with.
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
		})
		require.NoError(t, err)
	}()
	api := proto.NewDRPCProvisionerClient(drpcconn.New(client))

	for _, testCase := range []struct {
		Name     string
		Files    map[string]string
		Response *proto.Parse_Response
	}{{
		Name: "single-variable",
		Files: map[string]string{
			"main.tf": `variable "A" {
				description = "Testing!"
			}`,
		},
		Response: &proto.Parse_Response{
			ParameterSchemas: []*proto.ParameterSchema{{
				Name:        "A",
				Description: "Testing!",
			}},
		},
	}, {
		Name: "default-variable-value",
		Files: map[string]string{
			"main.tf": `variable "A" {
				default = "wow"
			}`,
		},
		Response: &proto.Parse_Response{
			ParameterSchemas: []*proto.ParameterSchema{{
				Name: "A",
				DefaultSource: &proto.ParameterSource{
					Scheme: proto.ParameterSource_DATA,
					Value:  "\"wow\"",
				},
				DefaultDestination: &proto.ParameterDestination{
					Scheme: proto.ParameterDestination_PROVISIONER_VARIABLE,
					Value:  "A",
				},
			}},
		},
	}, {
		Name: "variable-validation",
		Files: map[string]string{
			"main.tf": `variable "A" {
				validation {
					condition = var.A == "value"
				}
			}`,
		},
		Response: &proto.Parse_Response{
			ParameterSchemas: []*proto.ParameterSchema{{
				Name:                "A",
				ValidationCondition: `var.A == "value"`,
			}},
		},
	}} {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()

			// Write all files to the temporary test directory.
			directory := t.TempDir()
			for path, content := range testCase.Files {
				err := os.WriteFile(filepath.Join(directory, path), []byte(content), 0600)
				require.NoError(t, err)
			}

			response, err := api.Parse(ctx, &proto.Parse_Request{
				Directory: directory,
			})
			require.NoError(t, err)

			// Ensure the want and got are equivalent!
			want, err := json.Marshal(testCase.Response)
			require.NoError(t, err)
			got, err := json.Marshal(response)
			require.NoError(t, err)

			require.Equal(t, string(want), string(got))
		})
	}
}
