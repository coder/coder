package pulumi

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cli/safeexec"
	"github.com/hashicorp/go-version"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/jobreaper"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
)

var minPulumiVersion = version.Must(version.NewVersion("3.0.0"))

// ServeOptions are options for the Pulumi provisioner.
type ServeOptions struct {
	*provisionersdk.ServeOptions

	// BinaryPath specifies the "pulumi" binary to use.
	// If omitted, the $PATH will attempt to find it.
	BinaryPath string
	// CachePath stores Pulumi plugins and workspace state.
	CachePath string
	// ExitTimeout defines how long we will wait for a running Pulumi command
	// to exit cleanly if the provision was stopped.
	// Default: jobreaper.HungJobExitTimeout (3 minutes).
	ExitTimeout time.Duration
}

type server struct {
	execMut     *sync.Mutex
	logger      slog.Logger
	binaryPath  string
	cachePath   string
	exitTimeout time.Duration
}

func (s *server) executor(files tfpath.Layout, stage database.ProvisionerJobTimingStage) *executor {
	return &executor{
		server:     s,
		mut:        s.execMut,
		binaryPath: s.binaryPath,
		cachePath:  s.cachePath,
		files:      files,
		logger:     s.logger.Named("executor"),
		timings:    newTimingAggregator(stage),
	}
}

func (s *server) setupContexts(parentCtx context.Context, canceledOrComplete <-chan struct{}) (
	ctx context.Context, cancel func(), killCtx context.Context, kill func(),
) {
	ctx, cancel = context.WithCancel(parentCtx)
	killCtx, kill = context.WithCancel(context.Background())

	go func() {
		<-ctx.Done()
		s.logger.Debug(ctx, "graceful context done")

		t := time.NewTimer(s.exitTimeout)
		defer t.Stop()
		select {
		case <-t.C:
			s.logger.Debug(ctx, "exit timeout hit")
			kill()
		case <-killCtx.Done():
			s.logger.Debug(ctx, "kill context done")
		}
	}()

	go func() {
		<-canceledOrComplete
		s.logger.Debug(ctx, "canceledOrComplete closed")
		cancel()
	}()

	return ctx, cancel, killCtx, kill
}

var (
	_ = (*server).executor
	_ = (*server).setupContexts
)

// Serve starts a dRPC server on the provided transport speaking the Pulumi
// provisioner protocol.
func Serve(ctx context.Context, options *ServeOptions) error {
	if options == nil {
		return xerrors.New("options must not be nil")
	}
	if options.ServeOptions == nil {
		return xerrors.New("serve options must not be nil")
	}
	if options.ExitTimeout == 0 {
		options.ExitTimeout = jobreaper.HungJobExitTimeout
	}

	binaryPath, installedVersion, err := resolveBinary(ctx, options.BinaryPath)
	if err != nil {
		return err
	}
	options.BinaryPath = binaryPath
	options.Logger.Debug(ctx, "detected pulumi version",
		slog.F("binary_path", binaryPath),
		slog.F("installed_version", installedVersion.String()),
		slog.F("min_version", minPulumiVersion.String()),
	)

	return provisionersdk.Serve(ctx, &server{
		execMut:     &sync.Mutex{},
		logger:      options.Logger,
		binaryPath:  options.BinaryPath,
		cachePath:   options.CachePath,
		exitTimeout: options.ExitTimeout,
	}, options.ServeOptions)
}

func resolveBinary(ctx context.Context, configuredPath string) (string, *version.Version, error) {
	if ctx.Err() != nil {
		return "", nil, ctx.Err()
	}

	lookup := configuredPath
	if lookup == "" {
		lookup = "pulumi"
	}

	binaryPath, err := safeexec.LookPath(lookup)
	if err != nil {
		if configuredPath == "" {
			return "", nil, xerrors.Errorf("Pulumi binary not found: %w", err)
		}
		return "", nil, xerrors.Errorf("Pulumi binary %q not found: %w", configuredPath, err)
	}

	absoluteBinary, err := filepath.Abs(binaryPath)
	if err != nil {
		return "", nil, xerrors.Errorf("Pulumi binary absolute path not found: %w", err)
	}

	installedVersion, err := versionFromBinaryPath(ctx, absoluteBinary)
	if err != nil {
		return "", nil, xerrors.Errorf("Pulumi binary get version failed: %w", err)
	}
	if err := checkMinVersion(installedVersion); err != nil {
		return absoluteBinary, installedVersion, err
	}

	return absoluteBinary, installedVersion, nil
}

func versionFromBinaryPath(ctx context.Context, binaryPath string) (*version.Version, error) {
	if ctx == nil {
		return nil, xerrors.New("context must not be nil")
	}
	if binaryPath == "" {
		return nil, xerrors.New("binary path must not be empty")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// #nosec G204 -- Pulumi binary path is validated during serve startup.
	cmd := exec.CommandContext(ctx, binaryPath, "version")
	out, err := cmd.Output()
	if err != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return nil, err
		}
	}

	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) == 0 {
		return nil, xerrors.New("pulumi version output was empty")
	}

	parsedVersion, err := version.NewVersion(strings.TrimPrefix(fields[0], "v"))
	if err != nil {
		return nil, xerrors.Errorf("parse pulumi version %q: %w", fields[0], err)
	}
	return parsedVersion, nil
}

func checkMinVersion(installedVersion *version.Version) error {
	if installedVersion == nil {
		return xerrors.New("installed version must not be nil")
	}
	if installedVersion.LessThan(minPulumiVersion) {
		return xerrors.Errorf(
			"pulumi version %q is too old. required >= %q",
			installedVersion.String(),
			minPulumiVersion.String(),
		)
	}
	return nil
}

func interruptCommandOnCancel(ctx, killCtx context.Context, logger slog.Logger, cmd *exec.Cmd) {
	go func() {
		select {
		case <-ctx.Done():
			var err error
			switch runtime.GOOS {
			case "windows":
				err = cmd.Process.Kill()
			default:
				err = cmd.Process.Signal(os.Interrupt)
			}
			logger.Debug(ctx, "interrupted command", slog.F("args", cmd.Args), slog.Error(err))
		case <-killCtx.Done():
			logger.Debug(ctx, "kill context ended", slog.F("args", cmd.Args))
		}
	}()
}
