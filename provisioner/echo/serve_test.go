package echo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/drpc"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestEcho(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()

	// Create an in-memory provisioner to communicate with.
	client, server := drpc.MemTransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		cancelFunc()
	})
	go func() {
		err := echo.Serve(ctx, &provisionersdk.ServeOptions{
			Listener:      server,
			WorkDirectory: workdir,
		})
		assert.NoError(t, err)
	}()
	api := proto.NewDRPCProvisionerClient(client)

	t.Run("Parse", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
		defer cancel()

		responses := []*proto.Response{
			{
				Type: &proto.Response_Log{
					Log: &proto.Log{
						Output: "log-output",
					},
				},
			},
			{
				Type: &proto.Response_Parse{
					Parse: &proto.ParseComplete{},
				},
			},
		}
		data, err := echo.Tar(&echo.Responses{
			Parse: responses,
		})
		require.NoError(t, err)
		client, err := api.Session(ctx)
		require.NoError(t, err)
		defer func() {
			err := client.Close()
			require.NoError(t, err)
		}()
		err = client.Send(&proto.Request{Type: &proto.Request_Config{Config: &proto.Config{
			TemplateSourceArchive: data,
		}}})
		require.NoError(t, err)

		err = client.Send(&proto.Request{Type: &proto.Request_Parse{Parse: &proto.ParseRequest{}}})
		require.NoError(t, err)
		log, err := client.Recv()
		require.NoError(t, err)
		require.Equal(t, responses[0].GetLog().Output, log.GetLog().Output)
		complete, err := client.Recv()
		require.NoError(t, err)
		require.NotNil(t, complete)
	})

	t.Run("Provision", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
		defer cancel()

		planResponses := []*proto.Response{
			{
				Type: &proto.Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "log-output",
					},
				},
			},
			{
				Type: &proto.Response_Plan{
					Plan: &proto.PlanComplete{
						Resources: []*proto.Resource{{
							Name: "resource",
						}},
					},
				},
			},
		}
		applyResponses := []*proto.Response{
			{
				Type: &proto.Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: "log-output",
					},
				},
			},
			{
				Type: &proto.Response_Apply{
					Apply: &proto.ApplyComplete{
						Resources: []*proto.Resource{{
							Name: "resource",
						}},
					},
				},
			},
		}
		data, err := echo.Tar(&echo.Responses{
			ProvisionPlan:  planResponses,
			ProvisionApply: applyResponses,
		})
		require.NoError(t, err)
		client, err := api.Session(ctx)
		require.NoError(t, err)
		defer func() {
			err := client.Close()
			require.NoError(t, err)
		}()
		err = client.Send(&proto.Request{Type: &proto.Request_Config{Config: &proto.Config{
			TemplateSourceArchive: data,
		}}})
		require.NoError(t, err)

		err = client.Send(&proto.Request{Type: &proto.Request_Plan{Plan: &proto.PlanRequest{}}})
		require.NoError(t, err)
		log, err := client.Recv()
		require.NoError(t, err)
		require.Equal(t, planResponses[0].GetLog().Output, log.GetLog().Output)
		complete, err := client.Recv()
		require.NoError(t, err)
		require.Equal(t, planResponses[1].GetPlan().Resources[0].Name,
			complete.GetPlan().Resources[0].Name)

		err = client.Send(&proto.Request{Type: &proto.Request_Apply{Apply: &proto.ApplyRequest{}}})
		require.NoError(t, err)
		log, err = client.Recv()
		require.NoError(t, err)
		require.Equal(t, applyResponses[0].GetLog().Output, log.GetLog().Output)
		complete, err = client.Recv()
		require.NoError(t, err)
		require.Equal(t, applyResponses[1].GetApply().Resources[0].Name,
			complete.GetApply().Resources[0].Name)
	})

	t.Run("ProvisionStop", func(t *testing.T) {
		t.Parallel()

		// Stop responses should be returned when the workspace is being stopped.
		data, err := echo.Tar(&echo.Responses{
			ProvisionApply: applyCompleteResource("DEFAULT"),
			ProvisionPlan:  planCompleteResource("DEFAULT"),
			ProvisionPlanMap: map[proto.WorkspaceTransition][]*proto.Response{
				proto.WorkspaceTransition_STOP: planCompleteResource("STOP"),
			},
			ProvisionApplyMap: map[proto.WorkspaceTransition][]*proto.Response{
				proto.WorkspaceTransition_STOP: applyCompleteResource("STOP"),
			},
		})
		require.NoError(t, err)

		client, err := api.Session(ctx)
		require.NoError(t, err)
		defer func() {
			err := client.Close()
			require.NoError(t, err)
		}()
		err = client.Send(&proto.Request{Type: &proto.Request_Config{Config: &proto.Config{
			TemplateSourceArchive: data,
		}}})
		require.NoError(t, err)

		// Do stop.
		err = client.Send(&proto.Request{
			Type: &proto.Request_Plan{
				Plan: &proto.PlanRequest{
					Metadata: &proto.Metadata{
						WorkspaceTransition: proto.WorkspaceTransition_STOP,
					},
				},
			},
		})
		require.NoError(t, err)

		complete, err := client.Recv()
		require.NoError(t, err)
		require.Equal(t,
			"STOP",
			complete.GetPlan().Resources[0].Name,
		)

		// Do start.
		err = client.Send(&proto.Request{
			Type: &proto.Request_Plan{
				Plan: &proto.PlanRequest{
					Metadata: &proto.Metadata{
						WorkspaceTransition: proto.WorkspaceTransition_START,
					},
				},
			},
		})
		require.NoError(t, err)

		complete, err = client.Recv()
		require.NoError(t, err)
		require.Equal(t,
			"DEFAULT",
			complete.GetPlan().Resources[0].Name,
		)
	})

	t.Run("ProvisionWithLogLevel", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
		defer cancel()

		responses := []*proto.Response{{
			Type: &proto.Response_Log{
				Log: &proto.Log{
					Level:  proto.LogLevel_TRACE,
					Output: "log-output-trace",
				},
			},
		}, {
			Type: &proto.Response_Log{
				Log: &proto.Log{
					Level:  proto.LogLevel_INFO,
					Output: "log-output-info",
				},
			},
		}, {
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{
					Resources: []*proto.Resource{{
						Name: "resource",
					}},
				},
			},
		}}
		data, err := echo.Tar(&echo.Responses{
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: responses,
		})
		require.NoError(t, err)
		client, err := api.Session(ctx)
		require.NoError(t, err)
		defer func() {
			err := client.Close()
			require.NoError(t, err)
		}()
		err = client.Send(&proto.Request{Type: &proto.Request_Config{Config: &proto.Config{
			TemplateSourceArchive: data,
			ProvisionerLogLevel:   "debug",
		}}})
		require.NoError(t, err)

		// Plan is required before apply
		err = client.Send(&proto.Request{Type: &proto.Request_Plan{Plan: &proto.PlanRequest{}}})
		require.NoError(t, err)
		complete, err := client.Recv()
		require.NoError(t, err)
		require.NotNil(t, complete.GetPlan())

		err = client.Send(&proto.Request{Type: &proto.Request_Apply{Apply: &proto.ApplyRequest{}}})
		require.NoError(t, err)
		log, err := client.Recv()
		require.NoError(t, err)
		// Skip responses[0] as it's trace level
		require.Equal(t, responses[1].GetLog().Output, log.GetLog().Output)
		complete, err = client.Recv()
		require.NoError(t, err)
		require.Equal(t, responses[2].GetApply().Resources[0].Name,
			complete.GetApply().Resources[0].Name)
	})
}

func planCompleteResource(name string) []*proto.Response {
	return []*proto.Response{{
		Type: &proto.Response_Plan{
			Plan: &proto.PlanComplete{
				Resources: []*proto.Resource{{
					Name: name,
				}},
			},
		},
	}}
}

func applyCompleteResource(name string) []*proto.Response {
	return []*proto.Response{{
		Type: &proto.Response_Apply{
			Apply: &proto.ApplyComplete{
				Resources: []*proto.Resource{{
					Name: name,
				}},
			},
		},
	}}
}
