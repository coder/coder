package provisionerd_test

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/provisionerd/runner"
	"github.com/coder/coder/provisionersdk"
	sdkproto "github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func closedWithin(c chan struct{}, d time.Duration) func() bool {
	return func() bool {
		select {
		case <-c:
			return true
		case <-time.After(d):
			return false
		}
	}
}

func TestProvisionerd(t *testing.T) {
	t.Parallel()

	noopUpdateJob := func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
		return &proto.UpdateJobResponse{}, nil
	}

	t.Run("InstantClose", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{}), nil
		}, provisionerd.Provisioners{})
		require.NoError(t, closer.Close())
	})

	t.Run("ConnectErrorClose", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		completeChan := make(chan struct{})
		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			defer close(completeChan)
			return nil, xerrors.New("an error")
		}, provisionerd.Provisioners{})
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, closer.Close())
	})

	t.Run("AcquireEmptyJob", func(t *testing.T) {
		// The provisioner daemon is supposed to skip the job acquire if
		// the job provided is empty. This is to show it successfully
		// tried to get a job, but none were available.
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		completeChan := make(chan struct{})
		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			acquireJobAttempt := 0
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					if acquireJobAttempt == 1 {
						close(completeChan)
					}
					acquireJobAttempt++
					return &proto.AcquiredJob{}, nil
				},
				updateJob: noopUpdateJob,
			}), nil
		}, provisionerd.Provisioners{})
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, closer.Close())
	})

	t.Run("CloseCancelsJob", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		completeChan := make(chan struct{})
		var completed sync.Once
		var closer io.Closer
		var closerMutex sync.Mutex
		closerMutex.Lock()
		closer = createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					return &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_TemplateImport_{
							TemplateImport: &proto.AcquiredJob_TemplateImport{
								Metadata: &sdkproto.Provision_Metadata{},
							},
						},
					}, nil
				},
				updateJob: noopUpdateJob,
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					completed.Do(func() {
						close(completeChan)
					})
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.Provisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				parse: func(request *sdkproto.Parse_Request, stream sdkproto.DRPCProvisioner_ParseStream) error {
					closerMutex.Lock()
					defer closerMutex.Unlock()
					return closer.Close()
				},
			}),
		})
		closerMutex.Unlock()
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, closer.Close())
	})

	t.Run("MaliciousTar", func(t *testing.T) {
		// Ensures tars with "../../../etc/passwd" as the path
		// are not allowed to run, and will fail the job.
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		var (
			completeChan = make(chan struct{})
			completeOnce sync.Once
		)

		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					return &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"../../../etc/passwd": "content",
						}),
						Type: &proto.AcquiredJob_TemplateImport_{
							TemplateImport: &proto.AcquiredJob_TemplateImport{
								Metadata: &sdkproto.Provision_Metadata{},
							},
						},
					}, nil
				},
				updateJob: noopUpdateJob,
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					completeOnce.Do(func() { close(completeChan) })
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.Provisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{}),
		})
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, closer.Close())
	})

	t.Run("RunningPeriodicUpdate", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		var (
			completeChan = make(chan struct{})
			completeOnce sync.Once
		)

		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					return &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_TemplateImport_{
							TemplateImport: &proto.AcquiredJob_TemplateImport{
								Metadata: &sdkproto.Provision_Metadata{},
							},
						},
					}, nil
				},
				updateJob: func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
					completeOnce.Do(func() { close(completeChan) })
					return &proto.UpdateJobResponse{}, nil
				},
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.Provisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				parse: func(request *sdkproto.Parse_Request, stream sdkproto.DRPCProvisioner_ParseStream) error {
					<-stream.Context().Done()
					return nil
				},
			}),
		})
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, closer.Close())
	})

	t.Run("TemplateImport", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		var (
			didComplete   atomic.Bool
			didLog        atomic.Bool
			didAcquireJob atomic.Bool
			didDryRun     = atomic.NewBool(true)
			didReadme     atomic.Bool
			completeChan  = make(chan struct{})
			completeOnce  sync.Once
		)

		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					if !didAcquireJob.CAS(false, true) {
						completeOnce.Do(func() { close(completeChan) })
						return &proto.AcquiredJob{}, nil
					}

					return &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt":        "content",
							runner.ReadmeFile: "# A cool template 😎\n",
						}),
						Type: &proto.AcquiredJob_TemplateImport_{
							TemplateImport: &proto.AcquiredJob_TemplateImport{
								Metadata: &sdkproto.Provision_Metadata{},
							},
						},
					}, nil
				},
				updateJob: func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
					if len(update.Logs) > 0 {
						didLog.Store(true)
					}
					if len(update.Readme) > 0 {
						didReadme.Store(true)
					}
					return &proto.UpdateJobResponse{}, nil
				},
				completeJob: func(ctx context.Context, job *proto.CompletedJob) (*proto.Empty, error) {
					didComplete.Store(true)
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.Provisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				parse: func(request *sdkproto.Parse_Request, stream sdkproto.DRPCProvisioner_ParseStream) error {
					data, err := os.ReadFile(filepath.Join(request.Directory, "test.txt"))
					require.NoError(t, err)
					require.Equal(t, "content", string(data))

					err = stream.Send(&sdkproto.Parse_Response{
						Type: &sdkproto.Parse_Response_Log{
							Log: &sdkproto.Log{
								Level:  sdkproto.LogLevel_INFO,
								Output: "hello",
							},
						},
					})
					require.NoError(t, err)

					err = stream.Send(&sdkproto.Parse_Response{
						Type: &sdkproto.Parse_Response_Complete{
							Complete: &sdkproto.Parse_Complete{},
						},
					})
					require.NoError(t, err)
					return nil
				},
				provision: func(stream sdkproto.DRPCProvisioner_ProvisionStream) error {
					request, err := stream.Recv()
					require.NoError(t, err)
					if request.GetApply() != nil {
						didDryRun.Store(false)
					}
					err = stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Log{
							Log: &sdkproto.Log{
								Level:  sdkproto.LogLevel_INFO,
								Output: "hello",
							},
						},
					})
					require.NoError(t, err)

					err = stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Complete{
							Complete: &sdkproto.Provision_Complete{
								Resources: []*sdkproto.Resource{},
							},
						},
					})
					require.NoError(t, err)
					return nil
				},
			}),
		})

		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, closer.Close())
		assert.True(t, didLog.Load(), "should log some updates")
		assert.True(t, didComplete.Load(), "should complete the job")
		assert.True(t, didDryRun.Load(), "should be a dry run")
	})

	t.Run("TemplateDryRun", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		var (
			didComplete   atomic.Bool
			didLog        atomic.Bool
			didAcquireJob atomic.Bool
			completeChan  = make(chan struct{})
			completeOnce  sync.Once

			parameterValues = []*sdkproto.ParameterValue{
				{
					DestinationScheme: sdkproto.ParameterDestination_PROVISIONER_VARIABLE,
					Name:              "test_var",
					Value:             "dean was here",
				},
				{
					DestinationScheme: sdkproto.ParameterDestination_PROVISIONER_VARIABLE,
					Name:              "test_var_2",
					Value:             "1234",
				},
			}
			metadata = &sdkproto.Provision_Metadata{}
		)

		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					if !didAcquireJob.CAS(false, true) {
						completeOnce.Do(func() { close(completeChan) })
						return &proto.AcquiredJob{}, nil
					}

					return &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_TemplateDryRun_{
							TemplateDryRun: &proto.AcquiredJob_TemplateDryRun{
								ParameterValues: parameterValues,
								Metadata:        metadata,
							},
						},
					}, nil
				},
				updateJob: func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
					if len(update.Logs) == 0 {
						t.Log("provisionerDaemonTestServer: no log messages")
						return &proto.UpdateJobResponse{}, nil
					}

					didLog.Store(true)
					for _, msg := range update.Logs {
						t.Log("provisionerDaemonTestServer", "msg:", msg)
					}
					return &proto.UpdateJobResponse{}, nil
				},
				completeJob: func(ctx context.Context, job *proto.CompletedJob) (*proto.Empty, error) {
					didComplete.Store(true)
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.Provisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				provision: func(stream sdkproto.DRPCProvisioner_ProvisionStream) error {
					err := stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Complete{
							Complete: &sdkproto.Provision_Complete{
								Resources: []*sdkproto.Resource{},
							},
						},
					})
					require.NoError(t, err)
					return nil
				},
			}),
		})

		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, closer.Close())
		assert.True(t, didLog.Load(), "should log some updates")
		assert.True(t, didComplete.Load(), "should complete the job")
	})

	t.Run("WorkspaceBuild", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		var (
			didComplete   atomic.Bool
			didLog        atomic.Bool
			didAcquireJob atomic.Bool
			completeChan  = make(chan struct{})
			completeOnce  sync.Once
		)

		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					if !didAcquireJob.CAS(false, true) {
						completeOnce.Do(func() { close(completeChan) })
						return &proto.AcquiredJob{}, nil
					}

					return &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_WorkspaceBuild_{
							WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
								Metadata: &sdkproto.Provision_Metadata{},
							},
						},
					}, nil
				},
				updateJob: func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
					if len(update.Logs) != 0 {
						didLog.Store(true)
					}
					return &proto.UpdateJobResponse{}, nil
				},
				completeJob: func(ctx context.Context, job *proto.CompletedJob) (*proto.Empty, error) {
					didComplete.Store(true)
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.Provisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				provision: func(stream sdkproto.DRPCProvisioner_ProvisionStream) error {
					err := stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Log{
							Log: &sdkproto.Log{
								Level:  sdkproto.LogLevel_DEBUG,
								Output: "wow",
							},
						},
					})
					require.NoError(t, err)

					err = stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Complete{
							Complete: &sdkproto.Provision_Complete{},
						},
					})
					require.NoError(t, err)
					return nil
				},
			}),
		})
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, closer.Close())
		assert.True(t, didLog.Load(), "should log some updates")
		assert.True(t, didComplete.Load(), "should complete the job")
	})

	t.Run("WorkspaceBuildQuotaExceeded", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		var (
			didComplete   atomic.Bool
			didLog        atomic.Bool
			didAcquireJob atomic.Bool
			didFail       atomic.Bool
			completeChan  = make(chan struct{})
			completeOnce  sync.Once
		)

		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					if !didAcquireJob.CAS(false, true) {
						completeOnce.Do(func() { close(completeChan) })
						return &proto.AcquiredJob{}, nil
					}

					return &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_WorkspaceBuild_{
							WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
								Metadata: &sdkproto.Provision_Metadata{},
							},
						},
					}, nil
				},
				updateJob: func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
					if len(update.Logs) != 0 {
						didLog.Store(true)
					}
					return &proto.UpdateJobResponse{}, nil
				},
				completeJob: func(ctx context.Context, job *proto.CompletedJob) (*proto.Empty, error) {
					didComplete.Store(true)
					return &proto.Empty{}, nil
				},
				commitQuota: func(ctx context.Context, com *proto.CommitQuotaRequest) (*proto.CommitQuotaResponse, error) {
					return &proto.CommitQuotaResponse{
						Ok: com.DailyCost < 20,
					}, nil
				},
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					didFail.Store(true)
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.Provisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				provision: func(stream sdkproto.DRPCProvisioner_ProvisionStream) error {
					err := stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Log{
							Log: &sdkproto.Log{
								Level:  sdkproto.LogLevel_DEBUG,
								Output: "wow",
							},
						},
					})
					require.NoError(t, err)

					err = stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Complete{
							Complete: &sdkproto.Provision_Complete{
								Resources: []*sdkproto.Resource{
									{
										DailyCost: 10,
									},
									{
										DailyCost: 15,
									},
								},
							},
						},
					})
					require.NoError(t, err)
					return nil
				},
			}),
		})
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, closer.Close())
		assert.True(t, didLog.Load(), "should log some updates")
		assert.False(t, didComplete.Load(), "should complete the job")
		assert.True(t, didFail.Load(), "should fail the job")
	})

	t.Run("WorkspaceBuildFailComplete", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		var (
			didFail       atomic.Bool
			didAcquireJob atomic.Bool
			completeChan  = make(chan struct{})
			completeOnce  sync.Once
		)

		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					if !didAcquireJob.CAS(false, true) {
						completeOnce.Do(func() { close(completeChan) })
						return &proto.AcquiredJob{}, nil
					}

					return &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_WorkspaceBuild_{
							WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
								Metadata: &sdkproto.Provision_Metadata{},
							},
						},
					}, nil
				},
				updateJob: noopUpdateJob,
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					didFail.Store(true)
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.Provisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				provision: func(stream sdkproto.DRPCProvisioner_ProvisionStream) error {
					return stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Complete{
							Complete: &sdkproto.Provision_Complete{
								Error: "some error",
							},
						},
					})
				},
			}),
		})
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, closer.Close())
		assert.True(t, didFail.Load(), "should fail the job")
	})

	t.Run("Shutdown", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		var updated sync.Once
		var completed sync.Once
		updateChan := make(chan struct{})
		completeChan := make(chan struct{})
		server := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					return &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_WorkspaceBuild_{
							WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
								Metadata: &sdkproto.Provision_Metadata{},
							},
						},
					}, nil
				},
				updateJob: func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
					if len(update.Logs) > 0 {
						for _, log := range update.Logs {
							if log.Source != proto.LogSource_PROVISIONER {
								continue
							}
							// Close on a log so we know when the job is in progress!
							updated.Do(func() {
								close(updateChan)
							})
							break
						}
					}
					return &proto.UpdateJobResponse{}, nil
				},
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					completed.Do(func() {
						close(completeChan)
					})
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.Provisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				provision: func(stream sdkproto.DRPCProvisioner_ProvisionStream) error {
					// Ignore the first provision message!
					_, _ = stream.Recv()

					err := stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Log{
							Log: &sdkproto.Log{
								Level:  sdkproto.LogLevel_DEBUG,
								Output: "in progress",
							},
						},
					})
					require.NoError(t, err)

					msg, err := stream.Recv()
					require.NoError(t, err)
					require.NotNil(t, msg.GetCancel())

					return stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Complete{
							Complete: &sdkproto.Provision_Complete{
								Error: "some error",
							},
						},
					})
				},
			}),
		})
		require.Condition(t, closedWithin(updateChan, testutil.WaitShort))
		err := server.Shutdown(context.Background())
		require.NoError(t, err)
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, server.Close())
	})

	t.Run("ShutdownFromJob", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		var completed sync.Once
		var updated sync.Once
		updateChan := make(chan struct{})
		completeChan := make(chan struct{})
		server := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					return &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_WorkspaceBuild_{
							WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
								Metadata: &sdkproto.Provision_Metadata{},
							},
						},
					}, nil
				},
				updateJob: func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
					resp := &proto.UpdateJobResponse{}
					if len(update.Logs) > 0 {
						for _, log := range update.Logs {
							if log.Source != proto.LogSource_PROVISIONER {
								continue
							}
							// Close on a log so we know when the job is in progress!
							updated.Do(func() {
								close(updateChan)
							})
							break
						}
					}
					// start returning Canceled once we've gotten at least one log.
					select {
					case <-updateChan:
						resp.Canceled = true
					default:
						// pass
					}
					return resp, nil
				},
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					completed.Do(func() {
						close(completeChan)
					})
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.Provisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				provision: func(stream sdkproto.DRPCProvisioner_ProvisionStream) error {
					// Ignore the first provision message!
					_, _ = stream.Recv()

					err := stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Log{
							Log: &sdkproto.Log{
								Level:  sdkproto.LogLevel_DEBUG,
								Output: "in progress",
							},
						},
					})
					require.NoError(t, err)

					msg, err := stream.Recv()
					require.NoError(t, err)
					require.NotNil(t, msg.GetCancel())

					return stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Complete{
							Complete: &sdkproto.Provision_Complete{
								Error: "some error",
							},
						},
					})
				},
			}),
		})
		require.Condition(t, closedWithin(updateChan, testutil.WaitShort))
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, server.Close())
	})

	t.Run("ReconnectAndFail", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		var (
			second       atomic.Bool
			failChan     = make(chan struct{})
			failOnce     sync.Once
			failedChan   = make(chan struct{})
			failedOnce   sync.Once
			completeChan = make(chan struct{})
			completeOnce sync.Once
		)
		server := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			client := createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					if second.Load() {
						return &proto.AcquiredJob{}, nil
					}
					return &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_WorkspaceBuild_{
							WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
								Metadata: &sdkproto.Provision_Metadata{},
							},
						},
					}, nil
				},
				updateJob: func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
					return &proto.UpdateJobResponse{}, nil
				},
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					assert.Equal(t, job.JobId, "test")
					if second.Load() {
						completeOnce.Do(func() { close(completeChan) })
						return &proto.Empty{}, nil
					}
					failOnce.Do(func() { close(failChan) })
					<-failedChan
					return &proto.Empty{}, nil
				},
			})
			if !second.Load() {
				go func() {
					<-failChan
					_ = client.DRPCConn().Close()
					second.Store(true)
					time.Sleep(50 * time.Millisecond)
					failedOnce.Do(func() { close(failedChan) })
				}()
			}
			return client, nil
		}, provisionerd.Provisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				provision: func(stream sdkproto.DRPCProvisioner_ProvisionStream) error {
					// Ignore the first provision message!
					_, _ = stream.Recv()
					return stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Complete{
							Complete: &sdkproto.Provision_Complete{
								Error: "some error",
							},
						},
					})
				},
			}),
		})
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, server.Close())
	})

	t.Run("ReconnectAndComplete", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		var (
			second       atomic.Bool
			failChan     = make(chan struct{})
			failOnce     sync.Once
			failedChan   = make(chan struct{})
			failedOnce   sync.Once
			completeChan = make(chan struct{})
			completeOnce sync.Once
		)
		server := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			client := createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					if second.Load() {
						completeOnce.Do(func() { close(completeChan) })
						return &proto.AcquiredJob{}, nil
					}
					return &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_WorkspaceBuild_{
							WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
								Metadata: &sdkproto.Provision_Metadata{},
							},
						},
					}, nil
				},
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					return nil, yamux.ErrSessionShutdown
				},
				updateJob: func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
					return &proto.UpdateJobResponse{}, nil
				},
				completeJob: func(ctx context.Context, job *proto.CompletedJob) (*proto.Empty, error) {
					if second.Load() {
						return &proto.Empty{}, nil
					}
					failOnce.Do(func() { close(failChan) })
					<-failedChan
					return &proto.Empty{}, nil
				},
			})
			if !second.Load() {
				go func() {
					<-failChan
					_ = client.DRPCConn().Close()
					second.Store(true)
					time.Sleep(50 * time.Millisecond)
					failedOnce.Do(func() { close(failedChan) })
				}()
			}
			return client, nil
		}, provisionerd.Provisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				provision: func(stream sdkproto.DRPCProvisioner_ProvisionStream) error {
					// Ignore the first provision message!
					_, _ = stream.Recv()
					return stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Complete{
							Complete: &sdkproto.Provision_Complete{},
						},
					})
				},
			}),
		})
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, server.Close())
	})

	t.Run("UpdatesBeforeComplete", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		logger := slogtest.Make(t, nil)
		m := sync.Mutex{}
		var ops []string
		completeChan := make(chan struct{})
		completeOnce := sync.Once{}

		server := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJob: func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error) {
					m.Lock()
					defer m.Unlock()
					logger.Info(ctx, "AcquiredJob called.")
					if len(ops) > 0 {
						return &proto.AcquiredJob{}, nil
					}
					ops = append(ops, "AcquireJob")

					return &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_WorkspaceBuild_{
							WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
								Metadata: &sdkproto.Provision_Metadata{},
							},
						},
					}, nil
				},
				updateJob: func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
					m.Lock()
					defer m.Unlock()
					logger.Info(ctx, "UpdateJob called.")
					ops = append(ops, "UpdateJob")
					for _, log := range update.Logs {
						ops = append(ops, fmt.Sprintf("Log: %s | %s", log.Stage, log.Output))
					}
					return &proto.UpdateJobResponse{}, nil
				},
				completeJob: func(ctx context.Context, job *proto.CompletedJob) (*proto.Empty, error) {
					m.Lock()
					defer m.Unlock()
					logger.Info(ctx, "CompleteJob called.")
					ops = append(ops, "CompleteJob")
					completeOnce.Do(func() { close(completeChan) })
					return &proto.Empty{}, nil
				},
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					m.Lock()
					defer m.Unlock()
					logger.Info(ctx, "FailJob called.")
					ops = append(ops, "FailJob")
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.Provisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				provision: func(stream sdkproto.DRPCProvisioner_ProvisionStream) error {
					err := stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Log{
							Log: &sdkproto.Log{
								Level:  sdkproto.LogLevel_DEBUG,
								Output: "wow",
							},
						},
					})
					require.NoError(t, err)

					err = stream.Send(&sdkproto.Provision_Response{
						Type: &sdkproto.Provision_Response_Complete{
							Complete: &sdkproto.Provision_Complete{},
						},
					})
					require.NoError(t, err)
					return nil
				},
			}),
		})
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		require.NoError(t, server.Close())
		assert.Equal(t, ops[len(ops)-1], "CompleteJob")
		assert.Contains(t, ops[0:len(ops)-1], "Log: Cleaning Up | ")
	})
}

// Creates an in-memory tar of the files provided.
func createTar(t *testing.T, files map[string]string) []byte {
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for path, content := range files {
		err := writer.WriteHeader(&tar.Header{
			Name: path,
			Size: int64(len(content)),
		})
		require.NoError(t, err)

		_, err = writer.Write([]byte(content))
		require.NoError(t, err)
	}

	err := writer.Flush()
	require.NoError(t, err)
	return buffer.Bytes()
}

// Creates a provisionerd implementation with the provided dialer and provisioners.
func createProvisionerd(t *testing.T, dialer provisionerd.Dialer, provisioners provisionerd.Provisioners) *provisionerd.Server {
	server := provisionerd.New(dialer, &provisionerd.Options{
		Logger:          slogtest.Make(t, nil).Named("provisionerd").Leveled(slog.LevelDebug),
		JobPollInterval: 50 * time.Millisecond,
		UpdateInterval:  50 * time.Millisecond,
		Provisioners:    provisioners,
		WorkDirectory:   t.TempDir(),
	})
	t.Cleanup(func() {
		_ = server.Close()
	})
	return server
}

// Creates a provisionerd protobuf client that's connected
// to the server implementation provided.
func createProvisionerDaemonClient(t *testing.T, done <-chan struct{}, server provisionerDaemonTestServer) proto.DRPCProvisionerDaemonClient {
	t.Helper()
	if server.failJob == nil {
		// Default to asserting the error from the failure, otherwise
		// it can be lost in tests!
		server.failJob = func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
			assert.Fail(t, job.Error)
			return &proto.Empty{}, nil
		}
	}
	clientPipe, serverPipe := provisionersdk.MemTransportPipe()
	t.Cleanup(func() {
		_ = clientPipe.Close()
		_ = serverPipe.Close()
	})
	mux := drpcmux.New()
	err := proto.DRPCRegisterProvisionerDaemon(mux, &server)
	require.NoError(t, err)
	srv := drpcserver.New(mux)
	ctx, cancelFunc := context.WithCancel(context.Background())
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		_ = srv.Serve(ctx, serverPipe)
	}()
	t.Cleanup(func() {
		cancelFunc()
		<-closed
		select {
		case <-done:
			t.Error("createProvisionerDaemonClient cleanup after test was done!")
		default:
		}
	})
	select {
	case <-done:
		t.Error("called createProvisionerDaemonClient after test was done!")
	default:
	}
	return proto.NewDRPCProvisionerDaemonClient(clientPipe)
}

// Creates a provisioner protobuf client that's connected
// to the server implementation provided.
func createProvisionerClient(t *testing.T, done <-chan struct{}, server provisionerTestServer) sdkproto.DRPCProvisionerClient {
	t.Helper()
	clientPipe, serverPipe := provisionersdk.MemTransportPipe()
	t.Cleanup(func() {
		_ = clientPipe.Close()
		_ = serverPipe.Close()
	})
	mux := drpcmux.New()
	err := sdkproto.DRPCRegisterProvisioner(mux, &server)
	require.NoError(t, err)
	srv := drpcserver.New(mux)
	ctx, cancelFunc := context.WithCancel(context.Background())
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		_ = srv.Serve(ctx, serverPipe)
	}()
	t.Cleanup(func() {
		cancelFunc()
		<-closed
		select {
		case <-done:
			t.Error("createProvisionerClient cleanup after test was done!")
		default:
		}
	})
	select {
	case <-done:
		t.Error("called createProvisionerClient after test was done!")
	default:
	}
	return sdkproto.NewDRPCProvisionerClient(clientPipe)
}

type provisionerTestServer struct {
	parse     func(request *sdkproto.Parse_Request, stream sdkproto.DRPCProvisioner_ParseStream) error
	provision func(stream sdkproto.DRPCProvisioner_ProvisionStream) error
}

func (p *provisionerTestServer) Parse(request *sdkproto.Parse_Request, stream sdkproto.DRPCProvisioner_ParseStream) error {
	return p.parse(request, stream)
}

func (p *provisionerTestServer) Provision(stream sdkproto.DRPCProvisioner_ProvisionStream) error {
	return p.provision(stream)
}

// Fulfills the protobuf interface for a ProvisionerDaemon with
// passable functions for dynamic functionality.
type provisionerDaemonTestServer struct {
	acquireJob  func(ctx context.Context, _ *proto.Empty) (*proto.AcquiredJob, error)
	commitQuota func(ctx context.Context, com *proto.CommitQuotaRequest) (*proto.CommitQuotaResponse, error)
	updateJob   func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error)
	failJob     func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error)
	completeJob func(ctx context.Context, job *proto.CompletedJob) (*proto.Empty, error)
}

func (p *provisionerDaemonTestServer) AcquireJob(ctx context.Context, empty *proto.Empty) (*proto.AcquiredJob, error) {
	return p.acquireJob(ctx, empty)
}

func (p *provisionerDaemonTestServer) CommitQuota(ctx context.Context, com *proto.CommitQuotaRequest) (*proto.CommitQuotaResponse, error) {
	if p.commitQuota == nil {
		return &proto.CommitQuotaResponse{
			Ok: true,
		}, nil
	}
	return p.commitQuota(ctx, com)
}

func (p *provisionerDaemonTestServer) UpdateJob(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
	return p.updateJob(ctx, update)
}

func (p *provisionerDaemonTestServer) FailJob(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
	return p.failJob(ctx, job)
}

func (p *provisionerDaemonTestServer) CompleteJob(ctx context.Context, job *proto.CompletedJob) (*proto.Empty, error) {
	return p.completeJob(ctx, job)
}
