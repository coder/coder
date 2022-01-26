package provisionerd

import (
	"context"
	"sync"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/provisionerd/proto"
	provisionersdkproto "github.com/coder/coder/provisionersdk/proto"
)

type Dialer func(ctx context.Context) (proto.DRPCProvisionerDaemonClient, error)

type Options struct {
	Dialer Dialer
	Logger slog.Logger

	Provisioners map[string]provisionersdkproto.DRPCProvisionerClient
}

func New(dialer Dialer, opts *Options) {
}

type API struct {
	dialer Dialer
	opts   *Options

	acquireTicker time.Ticker
	apiClient     proto.DRPCProvisionerDaemonClient

	activeJob      *proto.AcquiredJob
	activeJobMutex sync.Mutex

	closed     chan struct{}
	closeMutex sync.Mutex
	closeError error

	jobStream proto.DRPCProvisionerDaemon_UpdateJobClient
}

func (s *API) init() {
	go func() {
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		for {
			select {
			case <-s.closed:
				return
			case <-s.acquireTicker.C:
				s.acquireJob(ctx)
			}
		}
	}()
}

func (s *API) acquireJob(ctx context.Context) {
	s.activeJobMutex.Lock()
	defer s.activeJobMutex.Unlock()
	if s.activeJob != nil {
		s.opts.Logger.Debug(ctx, "skipping job acquire. job is active")
		return
	}

	acquiredJob, err := s.apiClient.AcquireJob(ctx, &proto.Empty{})
	if err != nil {
		s.opts.Logger.Error(ctx, "acquire job", slog.Error(err))
		return
	}

	s.activeJob = acquiredJob
	// createdAt := time.UnixMilli(s.activeJob.CreatedAt)

}

func (s *API) hasActiveJob() bool {
	s.activeJobMutex.Lock()
	defer s.activeJobMutex.Unlock()
	return s.activeJob != nil
}

func (s *API) isClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *API) closeWithError(err error) error {
	s.closeMutex.Lock()
	defer s.closeMutex.Unlock()

	if s.isClosed() {
		return s.closeError
	}

	s.opts.Logger.Debug(context.Background(), "closing server with error", slog.Error(err))
	s.closeError = err
	close(s.closed)
	s.acquireTicker.Stop()

	return err
}

// func dial() {
// 	for r := retry.New(time.Second, time.Second*10); r.Wait(context.Background()); {
// 		// It should log on every connection retry.
// 	}

// 	var r proto.DRPCProvisionerDaemonClient
// 	stream, err := r.UpdateJob(context.Background())
// 	if err != nil {
// 		stream.Send(&proto.JobUpdate{
// 			Logs: []*proto.Log{{
// 				Source: proto.LogSource_DAEMON,
// 				Level: proto.LogLevel_INFO,
// 			}},
// 			JobId: ,
// 		})
// 	}
// 	logs, _ := r.JobLogs(context.Background())
// 	logs.Send()
// }
