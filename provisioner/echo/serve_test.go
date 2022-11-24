package echo_test

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestEcho(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	// Create an in-memory provisioner to communicate with.
	client, server := provisionersdk.MemTransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		cancelFunc()
	})
	go func() {
		err := echo.Serve(ctx, fs, &provisionersdk.ServeOptions{
			Listener: server,
		})
		assert.NoError(t, err)
	}()
	api := proto.NewDRPCProvisionerClient(client)

	t.Run("Parse", func(t *testing.T) {
		t.Parallel()

		responses := []*proto.Parse_Response{{
			Type: &proto.Parse_Response_Log{
				Log: &proto.Log{
					Output: "log-output",
				},
			},
		}, {
			Type: &proto.Parse_Response_Complete{
				Complete: &proto.Parse_Complete{
					ParameterSchemas: []*proto.ParameterSchema{{
						Name: "parameter-schema",
					}},
				},
			},
		}}
		data, err := echo.Tar(&echo.Responses{
			Parse: responses,
		})
		require.NoError(t, err)
		client, err := api.Parse(ctx, &proto.Parse_Request{
			Directory: unpackTar(t, fs, data),
		})
		require.NoError(t, err)
		log, err := client.Recv()
		require.NoError(t, err)
		require.Equal(t, responses[0].GetLog().Output, log.GetLog().Output)
		complete, err := client.Recv()
		require.NoError(t, err)
		require.Equal(t, responses[1].GetComplete().ParameterSchemas[0].Name,
			complete.GetComplete().ParameterSchemas[0].Name)
	})

	t.Run("Provision", func(t *testing.T) {
		t.Parallel()

		responses := []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Log{
				Log: &proto.Log{
					Output: "log-output",
				},
			},
		}, {
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "resource",
					}},
				},
			},
		}}
		data, err := echo.Tar(&echo.Responses{
			ProvisionApply: responses,
		})
		require.NoError(t, err)
		client, err := api.Provision(ctx)
		require.NoError(t, err)
		err = client.Send(&proto.Provision_Request{
			Type: &proto.Provision_Request_Plan{
				Plan: &proto.Provision_Plan{
					Config: &proto.Provision_Config{
						Directory: unpackTar(t, fs, data),
					},
				},
			},
		})
		require.NoError(t, err)
		log, err := client.Recv()
		require.NoError(t, err)
		require.Equal(t, responses[0].GetLog().Output, log.GetLog().Output)
		complete, err := client.Recv()
		require.NoError(t, err)
		require.Equal(t, responses[1].GetComplete().Resources[0].Name,
			complete.GetComplete().Resources[0].Name)
	})
}

func unpackTar(t *testing.T, fs afero.Fs, data []byte) string {
	directory := t.TempDir()
	reader := tar.NewReader(bytes.NewReader(data))
	for {
		header, err := reader.Next()
		if err != nil {
			break
		}
		// #nosec
		path := filepath.Join(directory, header.Name)
		file, err := fs.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		require.NoError(t, err)
		_, err = io.CopyN(file, reader, 1<<20)
		require.ErrorIs(t, err, io.EOF)
		err = file.Close()
		require.NoError(t, err)
	}
	return directory
}
