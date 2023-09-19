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
	"github.com/coder/coder/v2/provisionerd"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
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
		}, provisionerd.LocalProvisioners{})
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
		}, provisionerd.LocalProvisioners{})
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
				acquireJobWithCancel: func(stream proto.DRPCProvisionerDaemon_AcquireJobWithCancelStream) error {
					err := stream.Send(&proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_TemplateImport_{
							TemplateImport: &proto.AcquiredJob_TemplateImport{
								Metadata: &sdkproto.Metadata{},
							},
						},
					})
					assert.NoError(t, err)
					return nil
				},
				updateJob: noopUpdateJob,
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					completed.Do(func() {
						close(completeChan)
					})
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.LocalProvisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				parse: func(_ *provisionersdk.Session, _ *sdkproto.ParseRequest, _ <-chan struct{}) *sdkproto.ParseComplete {
					closerMutex.Lock()
					defer closerMutex.Unlock()
					err := closer.Close()
					c := &sdkproto.ParseComplete{}
					if err != nil {
						c.Error = err.Error()
					}
					return c
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
				acquireJobWithCancel: func(stream proto.DRPCProvisionerDaemon_AcquireJobWithCancelStream) error {
					err := stream.Send(&proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"../../../etc/passwd": "content",
						}),
						Type: &proto.AcquiredJob_TemplateImport_{
							TemplateImport: &proto.AcquiredJob_TemplateImport{
								Metadata: &sdkproto.Metadata{},
							},
						},
					})
					assert.NoError(t, err)
					return nil
				},
				updateJob: noopUpdateJob,
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					completeOnce.Do(func() { close(completeChan) })
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.LocalProvisioners{
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
				acquireJobWithCancel: func(stream proto.DRPCProvisionerDaemon_AcquireJobWithCancelStream) error {
					err := stream.Send(&proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_TemplateImport_{
							TemplateImport: &proto.AcquiredJob_TemplateImport{
								Metadata: &sdkproto.Metadata{},
							},
						},
					})
					assert.NoError(t, err)
					return nil
				},
				updateJob: func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
					completeOnce.Do(func() { close(completeChan) })
					return &proto.UpdateJobResponse{}, nil
				},
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.LocalProvisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				parse: func(
					_ *provisionersdk.Session,
					_ *sdkproto.ParseRequest,
					cancelOrComplete <-chan struct{},
				) *sdkproto.ParseComplete {
					<-cancelOrComplete
					return &sdkproto.ParseComplete{}
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
			didComplete atomic.Bool
			didLog      atomic.Bool
			didReadme   atomic.Bool
			acq         = newAcquireOne(t, &proto.AcquiredJob{
				JobId:       "test",
				Provisioner: "someprovisioner",
				TemplateSourceArchive: createTar(t, map[string]string{
					"test.txt":                "content",
					provisionersdk.ReadmeFile: "# A cool template 😎\n",
				}),
				Type: &proto.AcquiredJob_TemplateImport_{
					TemplateImport: &proto.AcquiredJob_TemplateImport{
						Metadata: &sdkproto.Metadata{},
					},
				},
			})
		)

		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJobWithCancel: acq.acquireWithCancel,
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
		}, provisionerd.LocalProvisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				parse: func(
					s *provisionersdk.Session,
					_ *sdkproto.ParseRequest,
					cancelOrComplete <-chan struct{},
				) *sdkproto.ParseComplete {
					data, err := os.ReadFile(filepath.Join(s.WorkDirectory, "test.txt"))
					require.NoError(t, err)
					require.Equal(t, "content", string(data))
					s.ProvisionLog(sdkproto.LogLevel_INFO, "hello")
					return &sdkproto.ParseComplete{}
				},
				plan: func(
					s *provisionersdk.Session,
					_ *sdkproto.PlanRequest,
					cancelOrComplete <-chan struct{},
				) *sdkproto.PlanComplete {
					s.ProvisionLog(sdkproto.LogLevel_INFO, "hello")
					return &sdkproto.PlanComplete{
						Resources: []*sdkproto.Resource{},
					}
				},
				apply: func(
					_ *provisionersdk.Session,
					_ *sdkproto.ApplyRequest,
					_ <-chan struct{},
				) *sdkproto.ApplyComplete {
					t.Error("dry run should not apply")
					return &sdkproto.ApplyComplete{}
				},
			}),
		})

		require.Condition(t, closedWithin(acq.complete, testutil.WaitShort))
		require.NoError(t, closer.Close())
		assert.True(t, didLog.Load(), "should log some updates")
		assert.True(t, didComplete.Load(), "should complete the job")
	})

	t.Run("TemplateDryRun", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		var (
			didComplete atomic.Bool
			didLog      atomic.Bool
			metadata    = &sdkproto.Metadata{}
			acq         = newAcquireOne(t, &proto.AcquiredJob{
				JobId:       "test",
				Provisioner: "someprovisioner",
				TemplateSourceArchive: createTar(t, map[string]string{
					"test.txt": "content",
				}),
				Type: &proto.AcquiredJob_TemplateDryRun_{
					TemplateDryRun: &proto.AcquiredJob_TemplateDryRun{
						Metadata: metadata,
					},
				},
			})
		)

		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJobWithCancel: acq.acquireWithCancel,
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
		}, provisionerd.LocalProvisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				plan: func(
					_ *provisionersdk.Session,
					_ *sdkproto.PlanRequest,
					_ <-chan struct{},
				) *sdkproto.PlanComplete {
					return &sdkproto.PlanComplete{
						Resources: []*sdkproto.Resource{},
					}
				},
				apply: func(
					_ *provisionersdk.Session,
					_ *sdkproto.ApplyRequest,
					_ <-chan struct{},
				) *sdkproto.ApplyComplete {
					t.Error("dry run should not apply")
					return &sdkproto.ApplyComplete{}
				},
			}),
		})

		require.Condition(t, closedWithin(acq.complete, testutil.WaitShort))
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
			didComplete atomic.Bool
			didLog      atomic.Bool
			acq         = newAcquireOne(t, &proto.AcquiredJob{
				JobId:       "test",
				Provisioner: "someprovisioner",
				TemplateSourceArchive: createTar(t, map[string]string{
					"test.txt": "content",
				}),
				Type: &proto.AcquiredJob_WorkspaceBuild_{
					WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
						Metadata: &sdkproto.Metadata{},
					},
				},
			})
		)

		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJobWithCancel: acq.acquireWithCancel,
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
		}, provisionerd.LocalProvisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				plan: func(
					s *provisionersdk.Session,
					_ *sdkproto.PlanRequest,
					cancelOrComplete <-chan struct{},
				) *sdkproto.PlanComplete {
					s.ProvisionLog(sdkproto.LogLevel_DEBUG, "wow")
					return &sdkproto.PlanComplete{}
				},
				apply: func(
					_ *provisionersdk.Session,
					_ *sdkproto.ApplyRequest,
					_ <-chan struct{},
				) *sdkproto.ApplyComplete {
					return &sdkproto.ApplyComplete{}
				},
			}),
		})
		require.Condition(t, closedWithin(acq.complete, testutil.WaitShort))
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
			didComplete atomic.Bool
			didLog      atomic.Bool
			didFail     atomic.Bool
			acq         = newAcquireOne(t, &proto.AcquiredJob{
				JobId:       "test",
				Provisioner: "someprovisioner",
				TemplateSourceArchive: createTar(t, map[string]string{
					"test.txt": "content",
				}),
				Type: &proto.AcquiredJob_WorkspaceBuild_{
					WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
						Metadata: &sdkproto.Metadata{},
					},
				},
			})
		)

		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJobWithCancel: acq.acquireWithCancel,
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
		}, provisionerd.LocalProvisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				plan: func(
					s *provisionersdk.Session,
					_ *sdkproto.PlanRequest,
					cancelOrComplete <-chan struct{},
				) *sdkproto.PlanComplete {
					s.ProvisionLog(sdkproto.LogLevel_DEBUG, "wow")
					return &sdkproto.PlanComplete{
						Resources: []*sdkproto.Resource{
							{
								DailyCost: 10,
							},
							{
								DailyCost: 15,
							},
						},
					}
				},
				apply: func(
					_ *provisionersdk.Session,
					_ *sdkproto.ApplyRequest,
					_ <-chan struct{},
				) *sdkproto.ApplyComplete {
					t.Error("should not apply when resources exceed quota")
					return &sdkproto.ApplyComplete{
						Resources: []*sdkproto.Resource{
							{
								DailyCost: 10,
							},
							{
								DailyCost: 15,
							},
						},
					}
				},
			}),
		})
		require.Condition(t, closedWithin(acq.complete, testutil.WaitShort))
		require.NoError(t, closer.Close())
		assert.True(t, didLog.Load(), "should log some updates")
		assert.False(t, didComplete.Load(), "should not complete the job")
		assert.True(t, didFail.Load(), "should fail the job")
	})

	t.Run("WorkspaceBuildFailComplete", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		var (
			didFail atomic.Bool
			acq     = newAcquireOne(t, &proto.AcquiredJob{
				JobId:       "test",
				Provisioner: "someprovisioner",
				TemplateSourceArchive: createTar(t, map[string]string{
					"test.txt": "content",
				}),
				Type: &proto.AcquiredJob_WorkspaceBuild_{
					WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
						Metadata: &sdkproto.Metadata{},
					},
				},
			})
		)

		closer := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJobWithCancel: acq.acquireWithCancel,
				updateJob:            noopUpdateJob,
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					didFail.Store(true)
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.LocalProvisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				plan: func(
					s *provisionersdk.Session,
					_ *sdkproto.PlanRequest,
					cancelOrComplete <-chan struct{},
				) *sdkproto.PlanComplete {
					return &sdkproto.PlanComplete{
						Error: "some error",
					}
				},
				apply: func(
					_ *provisionersdk.Session,
					_ *sdkproto.ApplyRequest,
					_ <-chan struct{},
				) *sdkproto.ApplyComplete {
					t.Error("should not apply when plan errors")
					return &sdkproto.ApplyComplete{
						Error: "some error",
					}
				},
			}),
		})
		require.Condition(t, closedWithin(acq.complete, testutil.WaitShort))
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
				acquireJobWithCancel: func(stream proto.DRPCProvisionerDaemon_AcquireJobWithCancelStream) error {
					err := stream.Send(&proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_WorkspaceBuild_{
							WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
								Metadata: &sdkproto.Metadata{},
							},
						},
					})
					assert.NoError(t, err)
					return nil
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
		}, provisionerd.LocalProvisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				plan: func(
					s *provisionersdk.Session,
					_ *sdkproto.PlanRequest,
					canceledOrComplete <-chan struct{},
				) *sdkproto.PlanComplete {
					s.ProvisionLog(sdkproto.LogLevel_DEBUG, "in progress")
					<-canceledOrComplete
					return &sdkproto.PlanComplete{
						Error: "some error",
					}
				},
				apply: func(
					_ *provisionersdk.Session,
					_ *sdkproto.ApplyRequest,
					_ <-chan struct{},
				) *sdkproto.ApplyComplete {
					t.Error("should not apply when shut down during plan")
					return &sdkproto.ApplyComplete{}
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
				acquireJobWithCancel: func(stream proto.DRPCProvisionerDaemon_AcquireJobWithCancelStream) error {
					err := stream.Send(&proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_WorkspaceBuild_{
							WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
								Metadata: &sdkproto.Metadata{},
							},
						},
					})
					assert.NoError(t, err)
					return nil
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
		}, provisionerd.LocalProvisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				plan: func(
					s *provisionersdk.Session,
					_ *sdkproto.PlanRequest,
					canceledOrComplete <-chan struct{},
				) *sdkproto.PlanComplete {
					s.ProvisionLog(sdkproto.LogLevel_DEBUG, "in progress")
					<-canceledOrComplete
					return &sdkproto.PlanComplete{
						Error: "some error",
					}
				},
				apply: func(
					_ *provisionersdk.Session,
					_ *sdkproto.ApplyRequest,
					_ <-chan struct{},
				) *sdkproto.ApplyComplete {
					t.Error("should not apply when shut down during plan")
					return &sdkproto.ApplyComplete{}
				},
			}),
		})
		require.Condition(t, closedWithin(updateChan, testutil.WaitShort))
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		require.NoError(t, server.Shutdown(ctx))
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
				acquireJobWithCancel: func(stream proto.DRPCProvisionerDaemon_AcquireJobWithCancelStream) error {
					job := &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_WorkspaceBuild_{
							WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
								Metadata: &sdkproto.Metadata{},
							},
						},
					}
					if second.Load() {
						job = &proto.AcquiredJob{}
						_, err := stream.Recv()
						assert.NoError(t, err)
					}
					err := stream.Send(job)
					assert.NoError(t, err)
					return nil
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
		}, provisionerd.LocalProvisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				plan: func(
					_ *provisionersdk.Session,
					_ *sdkproto.PlanRequest,
					_ <-chan struct{},
				) *sdkproto.PlanComplete {
					return &sdkproto.PlanComplete{
						Error: "some error",
					}
				},
				apply: func(
					_ *provisionersdk.Session,
					_ *sdkproto.ApplyRequest,
					_ <-chan struct{},
				) *sdkproto.ApplyComplete {
					t.Error("should not apply when error during plan")
					return &sdkproto.ApplyComplete{}
				},
			}),
		})
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		require.NoError(t, server.Shutdown(ctx))
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
				acquireJobWithCancel: func(stream proto.DRPCProvisionerDaemon_AcquireJobWithCancelStream) error {
					if second.Load() {
						completeOnce.Do(func() { close(completeChan) })
						_, err := stream.Recv()
						assert.NoError(t, err)
						return nil
					}
					job := &proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_WorkspaceBuild_{
							WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
								Metadata: &sdkproto.Metadata{},
							},
						},
					}
					err := stream.Send(job)
					assert.NoError(t, err)
					return nil
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
		}, provisionerd.LocalProvisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				plan: func(
					_ *provisionersdk.Session,
					_ *sdkproto.PlanRequest,
					_ <-chan struct{},
				) *sdkproto.PlanComplete {
					return &sdkproto.PlanComplete{}
				},
				apply: func(
					_ *provisionersdk.Session,
					_ *sdkproto.ApplyRequest,
					_ <-chan struct{},
				) *sdkproto.ApplyComplete {
					return &sdkproto.ApplyComplete{}
				},
			}),
		})
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		t.Log("completeChan closed")
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		require.NoError(t, server.Shutdown(ctx))
		require.NoError(t, server.Close())
	})

	t.Run("UpdatesBeforeComplete", func(t *testing.T) {
		t.Parallel()
		done := make(chan struct{})
		t.Cleanup(func() {
			close(done)
		})
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		m := sync.Mutex{}
		var ops []string
		completeChan := make(chan struct{})
		completeOnce := sync.Once{}

		server := createProvisionerd(t, func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
			return createProvisionerDaemonClient(t, done, provisionerDaemonTestServer{
				acquireJobWithCancel: func(stream proto.DRPCProvisionerDaemon_AcquireJobWithCancelStream) error {
					m.Lock()
					defer m.Unlock()
					logger.Info(ctx, "provisioner stage: AcquiredJob")
					if len(ops) > 0 {
						_, err := stream.Recv()
						assert.NoError(t, err)
						err = stream.Send(&proto.AcquiredJob{})
						assert.NoError(t, err)
						return nil
					}
					ops = append(ops, "AcquireJob")

					err := stream.Send(&proto.AcquiredJob{
						JobId:       "test",
						Provisioner: "someprovisioner",
						TemplateSourceArchive: createTar(t, map[string]string{
							"test.txt": "content",
						}),
						Type: &proto.AcquiredJob_WorkspaceBuild_{
							WorkspaceBuild: &proto.AcquiredJob_WorkspaceBuild{
								Metadata: &sdkproto.Metadata{},
							},
						},
					})
					assert.NoError(t, err)
					return nil
				},
				updateJob: func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error) {
					m.Lock()
					defer m.Unlock()
					logger.Info(ctx, "provisioner stage: UpdateJob")
					ops = append(ops, "UpdateJob")
					for _, log := range update.Logs {
						ops = append(ops, fmt.Sprintf("Log: %s | %s", log.Stage, log.Output))
					}
					return &proto.UpdateJobResponse{}, nil
				},
				completeJob: func(ctx context.Context, job *proto.CompletedJob) (*proto.Empty, error) {
					m.Lock()
					defer m.Unlock()
					logger.Info(ctx, "provisioner stage: CompleteJob")
					ops = append(ops, "CompleteJob")
					completeOnce.Do(func() { close(completeChan) })
					return &proto.Empty{}, nil
				},
				failJob: func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error) {
					m.Lock()
					defer m.Unlock()
					logger.Info(ctx, "provisioner stage: FailJob")
					ops = append(ops, "FailJob")
					return &proto.Empty{}, nil
				},
			}), nil
		}, provisionerd.LocalProvisioners{
			"someprovisioner": createProvisionerClient(t, done, provisionerTestServer{
				plan: func(
					s *provisionersdk.Session,
					_ *sdkproto.PlanRequest,
					_ <-chan struct{},
				) *sdkproto.PlanComplete {
					s.ProvisionLog(sdkproto.LogLevel_DEBUG, "wow")
					return &sdkproto.PlanComplete{}
				},
				apply: func(
					s *provisionersdk.Session,
					_ *sdkproto.ApplyRequest,
					_ <-chan struct{},
				) *sdkproto.ApplyComplete {
					s.ProvisionLog(sdkproto.LogLevel_DEBUG, "wow")
					return &sdkproto.ApplyComplete{}
				},
			}),
		})
		require.Condition(t, closedWithin(completeChan, testutil.WaitShort))
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		require.NoError(t, server.Shutdown(ctx))
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
func createProvisionerd(t *testing.T, dialer provisionerd.Dialer, connector provisionerd.LocalProvisioners) *provisionerd.Server {
	server := provisionerd.New(dialer, &provisionerd.Options{
		Logger:         slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named("provisionerd").Leveled(slog.LevelDebug),
		UpdateInterval: 50 * time.Millisecond,
		Connector:      connector,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		_ = server.Shutdown(ctx)
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
	ctx, cancelFunc := context.WithCancel(context.Background())
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		_ = provisionersdk.Serve(ctx, &server, &provisionersdk.ServeOptions{
			Listener:      serverPipe,
			Logger:        slogtest.Make(t, nil).Leveled(slog.LevelDebug).Named("test-provisioner"),
			WorkDirectory: t.TempDir(),
		})
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
	parse func(s *provisionersdk.Session, r *sdkproto.ParseRequest, canceledOrComplete <-chan struct{}) *sdkproto.ParseComplete
	plan  func(s *provisionersdk.Session, r *sdkproto.PlanRequest, canceledOrComplete <-chan struct{}) *sdkproto.PlanComplete
	apply func(s *provisionersdk.Session, r *sdkproto.ApplyRequest, canceledOrComplete <-chan struct{}) *sdkproto.ApplyComplete
}

func (p *provisionerTestServer) Parse(s *provisionersdk.Session, r *sdkproto.ParseRequest, canceledOrComplete <-chan struct{}) *sdkproto.ParseComplete {
	return p.parse(s, r, canceledOrComplete)
}

func (p *provisionerTestServer) Plan(s *provisionersdk.Session, r *sdkproto.PlanRequest, canceledOrComplete <-chan struct{}) *sdkproto.PlanComplete {
	return p.plan(s, r, canceledOrComplete)
}

func (p *provisionerTestServer) Apply(s *provisionersdk.Session, r *sdkproto.ApplyRequest, canceledOrComplete <-chan struct{}) *sdkproto.ApplyComplete {
	return p.apply(s, r, canceledOrComplete)
}

// Fulfills the protobuf interface for a ProvisionerDaemon with
// passable functions for dynamic functionality.
type provisionerDaemonTestServer struct {
	acquireJobWithCancel func(stream proto.DRPCProvisionerDaemon_AcquireJobWithCancelStream) error
	commitQuota          func(ctx context.Context, com *proto.CommitQuotaRequest) (*proto.CommitQuotaResponse, error)
	updateJob            func(ctx context.Context, update *proto.UpdateJobRequest) (*proto.UpdateJobResponse, error)
	failJob              func(ctx context.Context, job *proto.FailedJob) (*proto.Empty, error)
	completeJob          func(ctx context.Context, job *proto.CompletedJob) (*proto.Empty, error)
}

func (*provisionerDaemonTestServer) AcquireJob(context.Context, *proto.Empty) (*proto.AcquiredJob, error) {
	return nil, xerrors.New("deprecated!")
}

func (p *provisionerDaemonTestServer) AcquireJobWithCancel(stream proto.DRPCProvisionerDaemon_AcquireJobWithCancelStream) error {
	if p.acquireJobWithCancel != nil {
		return p.acquireJobWithCancel(stream)
	}
	// default behavior is to wait for cancel
	_, _ = stream.Recv()
	_ = stream.Send(&proto.AcquiredJob{})
	return nil
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

// acquireOne provides a function that returns a single provisioner job, then subsequent calls block until canceled.
// The complete channel is closed on the 2nd call.
type acquireOne struct {
	t        *testing.T
	mu       sync.Mutex
	job      *proto.AcquiredJob
	called   int
	complete chan struct{}
}

func newAcquireOne(t *testing.T, job *proto.AcquiredJob) *acquireOne {
	return &acquireOne{
		t:        t,
		job:      job,
		complete: make(chan struct{}),
	}
}

func (a *acquireOne) acquireWithCancel(stream proto.DRPCProvisionerDaemon_AcquireJobWithCancelStream) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.called++
	if a.called == 2 {
		close(a.complete)
	}
	if a.called > 1 {
		_, _ = stream.Recv()
		_ = stream.Send(&proto.AcquiredJob{})
		return nil
	}
	err := stream.Send(a.job)
	assert.NoError(a.t, err)
	return nil
}
