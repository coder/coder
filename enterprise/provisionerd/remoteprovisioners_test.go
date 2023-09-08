package provisionerd_test

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/enterprise/provisionerd"
	"github.com/coder/coder/v2/provisioner/echo"
	agpl "github.com/coder/coder/v2/provisionerd"
	"github.com/coder/coder/v2/provisionerd/proto"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRemoteConnector_Mainline(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		smokescreen bool
	}{
		{name: "NoSmokescreen", smokescreen: false},
		{name: "Smokescreen", smokescreen: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
			defer cancel()
			logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			exec := &testExecutor{
				t:           t,
				logger:      logger,
				smokescreen: tc.smokescreen,
			}
			uut, err := provisionerd.NewRemoteConnector(ctx, logger.Named("connector"), exec)
			require.NoError(t, err)

			respCh := make(chan agpl.ConnectResponse)
			job := &proto.AcquiredJob{
				JobId:       "test-job",
				Provisioner: string(database.ProvisionerTypeEcho),
			}
			uut.Connect(ctx, job, respCh)
			var resp agpl.ConnectResponse
			select {
			case <-ctx.Done():
				t.Error("timeout waiting for connect response")
			case resp = <-respCh:
				// OK
			}
			require.NoError(t, resp.Error)
			require.Equal(t, job, resp.Job)
			require.NotNil(t, resp.Client)

			// check that we can communicate with the provisioner
			er := &echo.Responses{
				Parse:          echo.ParseComplete,
				ProvisionApply: echo.ApplyComplete,
				ProvisionPlan:  echo.PlanComplete,
			}
			arc, err := echo.Tar(er)
			require.NoError(t, err)
			c := resp.Client
			s, err := c.Session(ctx)
			require.NoError(t, err)
			err = s.Send(&sdkproto.Request{Type: &sdkproto.Request_Config{Config: &sdkproto.Config{
				TemplateSourceArchive: arc,
			}}})
			require.NoError(t, err)
			err = s.Send(&sdkproto.Request{Type: &sdkproto.Request_Parse{Parse: &sdkproto.ParseRequest{}}})
			require.NoError(t, err)
			r, err := s.Recv()
			require.NoError(t, err)
			require.IsType(t, &sdkproto.Response_Parse{}, r.Type)
		})
	}
}

func TestRemoteConnector_BadToken(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	exec := &testExecutor{
		t:             t,
		logger:        logger,
		overrideToken: "bad-token",
	}
	uut, err := provisionerd.NewRemoteConnector(ctx, logger.Named("connector"), exec)
	require.NoError(t, err)

	respCh := make(chan agpl.ConnectResponse)
	job := &proto.AcquiredJob{
		JobId:       "test-job",
		Provisioner: string(database.ProvisionerTypeEcho),
	}
	uut.Connect(ctx, job, respCh)
	var resp agpl.ConnectResponse
	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for connect response")
	case resp = <-respCh:
		// OK
	}
	require.Equal(t, job, resp.Job)
	require.ErrorContains(t, resp.Error, "invalid token")
}

func TestRemoteConnector_BadJobID(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	exec := &testExecutor{
		t:             t,
		logger:        logger,
		overrideJobID: "bad-job",
	}
	uut, err := provisionerd.NewRemoteConnector(ctx, logger.Named("connector"), exec)
	require.NoError(t, err)

	respCh := make(chan agpl.ConnectResponse)
	job := &proto.AcquiredJob{
		JobId:       "test-job",
		Provisioner: string(database.ProvisionerTypeEcho),
	}
	uut.Connect(ctx, job, respCh)
	var resp agpl.ConnectResponse
	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for connect response")
	case resp = <-respCh:
		// OK
	}
	require.Equal(t, job, resp.Job)
	require.ErrorContains(t, resp.Error, "invalid job ID")
}

func TestRemoteConnector_BadCert(t *testing.T) {
	t.Parallel()
	_, cert, err := provisionerd.GenCert()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	exec := &testExecutor{
		t:            t,
		logger:       logger,
		overrideCert: string(cert),
	}
	uut, err := provisionerd.NewRemoteConnector(ctx, logger.Named("connector"), exec)
	require.NoError(t, err)

	respCh := make(chan agpl.ConnectResponse)
	job := &proto.AcquiredJob{
		JobId:       "test-job",
		Provisioner: string(database.ProvisionerTypeEcho),
	}
	uut.Connect(ctx, job, respCh)
	var resp agpl.ConnectResponse
	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for connect response")
	case resp = <-respCh:
		// OK
	}
	require.Equal(t, job, resp.Job)
	require.ErrorContains(t, resp.Error, "certificate signed by unknown authority")
}

func TestRemoteConnector_Fuzz(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	exec := newFuzzExecutor(t, logger)
	uut, err := provisionerd.NewRemoteConnector(ctx, logger.Named("connector"), exec)
	require.NoError(t, err)

	respCh := make(chan agpl.ConnectResponse)
	job := &proto.AcquiredJob{
		JobId:       "test-job",
		Provisioner: string(database.ProvisionerTypeEcho),
	}

	connectCtx, connectCtxCancel := context.WithCancel(ctx)
	defer connectCtxCancel()

	uut.Connect(connectCtx, job, respCh)
	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for fuzzer")
	case <-exec.done:
		// Connector hung up on the fuzzer
	}
	require.Less(t, exec.bytesFuzzed, 2<<20, "should not allow more than 1 MiB")
	connectCtxCancel()
	var resp agpl.ConnectResponse
	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for connect response")
	case resp = <-respCh:
		// OK
	}
	require.Equal(t, job, resp.Job)
	require.ErrorIs(t, resp.Error, context.Canceled)
}

func TestRemoteConnector_CancelConnect(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	exec := &testExecutor{
		t:         t,
		logger:    logger,
		dontStart: true,
	}
	uut, err := provisionerd.NewRemoteConnector(ctx, logger.Named("connector"), exec)
	require.NoError(t, err)

	respCh := make(chan agpl.ConnectResponse)
	job := &proto.AcquiredJob{
		JobId:       "test-job",
		Provisioner: string(database.ProvisionerTypeEcho),
	}

	connectCtx, connectCtxCancel := context.WithCancel(ctx)
	defer connectCtxCancel()

	uut.Connect(connectCtx, job, respCh)
	connectCtxCancel()
	var resp agpl.ConnectResponse
	select {
	case <-ctx.Done():
		t.Fatal("timeout waiting for connect response")
	case resp = <-respCh:
		// OK
	}
	require.Equal(t, job, resp.Job)
	require.ErrorIs(t, resp.Error, context.Canceled)
}

type testExecutor struct {
	t             *testing.T
	logger        slog.Logger
	overrideToken string
	overrideJobID string
	overrideCert  string
	// dontStart simulates when everything looks good to the connector but
	// the provisioner never starts
	dontStart bool
	// smokescreen starts a connection that fails authentication before starting
	// the real connection.  Tests that failed connections don't interfere with
	// real ones.
	smokescreen bool
}

func (e *testExecutor) Execute(
	ctx context.Context,
	provisionerType database.ProvisionerType,
	jobID, token, daemonCert, daemonAddress string,
) <-chan error {
	assert.Equal(e.t, database.ProvisionerTypeEcho, provisionerType)
	if e.overrideToken != "" {
		token = e.overrideToken
	}
	if e.overrideJobID != "" {
		jobID = e.overrideJobID
	}
	if e.overrideCert != "" {
		daemonCert = e.overrideCert
	}
	cacheDir := e.t.TempDir()
	errCh := make(chan error)
	go func() {
		defer close(errCh)
		if e.smokescreen {
			e.doSmokeScreen(ctx, jobID, daemonCert, daemonAddress)
		}
		if !e.dontStart {
			err := provisionerd.EphemeralEcho(ctx, e.logger, cacheDir, jobID, token, daemonCert, daemonAddress)
			e.logger.Debug(ctx, "provisioner done", slog.Error(err))
			if err != nil {
				errCh <- err
			}
		}
	}()
	return errCh
}

func (e *testExecutor) doSmokeScreen(ctx context.Context, jobID, daemonCert, daemonAddress string) {
	conn, err := provisionerd.DialTLS(ctx, daemonCert, daemonAddress)
	if !assert.NoError(e.t, err) {
		return
	}
	defer conn.Close()
	err = provisionerd.AuthenticateProvisioner(conn, "smokescreen", jobID)
	assert.ErrorContains(e.t, err, "invalid token")
}

type fuzzExecutor struct {
	t           *testing.T
	logger      slog.Logger
	done        chan struct{}
	bytesFuzzed int
}

func newFuzzExecutor(t *testing.T, logger slog.Logger) *fuzzExecutor {
	return &fuzzExecutor{
		t:           t,
		logger:      logger,
		done:        make(chan struct{}),
		bytesFuzzed: 0,
	}
}

func (e *fuzzExecutor) Execute(
	ctx context.Context,
	_ database.ProvisionerType,
	_, _, daemonCert, daemonAddress string,
) <-chan error {
	errCh := make(chan error)
	go func() {
		defer close(errCh)
		defer close(e.done)
		conn, err := provisionerd.DialTLS(ctx, daemonCert, daemonAddress)
		assert.NoError(e.t, err)
		rb := make([]byte, 128)
		for {
			if ctx.Err() != nil {
				e.t.Error("context canceled while fuzzing")
				return
			}
			n, err := rand.Read(rb)
			if err != nil {
				e.t.Errorf("random read: %s", err)
			}
			if n < 128 {
				e.t.Error("short random read")
				return
			}
			// replace newlines so the Connector doesn't think we are done
			// with the JobID
			for i := 0; i < len(rb); i++ {
				if rb[i] == '\n' || rb[i] == '\r' {
					rb[i] = 'A'
				}
			}
			n, err = conn.Write(rb)
			e.bytesFuzzed += n
			if err != nil {
				return
			}
		}
	}()
	return errCh
}
