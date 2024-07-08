package terraform

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/cli/safeexec"
	semconv "go.opentelemetry.io/otel/semconv/v1.14.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/unhanger"
	"github.com/coder/coder/v2/provisionersdk"
)

type ServeOptions struct {
	*provisionersdk.ServeOptions

	// BinaryPath specifies the "terraform" binary to use.
	// If omitted, the $PATH will attempt to find it.
	BinaryPath string
	// CachePath must not be used by multiple processes at once.
	CachePath string
	Tracer    trace.Tracer

	// ExitTimeout defines how long we will wait for a running Terraform
	// command to exit (cleanly) if the provision was stopped. This
	// happens when the provision is canceled via RPC and when the command is
	// still running after the provision stream is closed.
	//
	// This is a no-op on Windows where the process can't be interrupted.
	//
	// Default value: 3 minutes (unhanger.HungJobExitTimeout). This value should
	// be kept less than the value that Coder uses to mark hung jobs as failed,
	// which is 5 minutes (see unhanger package).
	ExitTimeout time.Duration
}

func absoluteBinaryPath(ctx context.Context, logger slog.Logger) (string, error) {
	binaryPath, err := safeexec.LookPath("terraform")
	if err != nil {
		return "", xerrors.Errorf("Terraform binary not found: %w", err)
	}

	// If the "coder" binary is in the same directory as
	// the "terraform" binary, "terraform" is returned.
	//
	// We must resolve the absolute path for other processes
	// to execute this properly!
	absoluteBinary, err := filepath.Abs(binaryPath)
	if err != nil {
		return "", xerrors.Errorf("Terraform binary absolute path not found: %w", err)
	}

	// Checking the installed version of Terraform.
	installedVersion, err := versionFromBinaryPath(ctx, absoluteBinary)
	if err != nil {
		return "", xerrors.Errorf("Terraform binary get version failed: %w", err)
	}

	logger.Info(ctx, "detected terraform version",
		slog.F("installed_version", installedVersion.String()),
		slog.F("min_version", minTerraformVersion.String()),
		slog.F("max_version", maxTerraformVersion.String()))

	if installedVersion.LessThan(minTerraformVersion) {
		logger.Warn(ctx, "installed terraform version too old, will download known good version to cache")
		return "", terraformMinorVersionMismatch
	}

	// Warn if the installed version is newer than what we've decided is the max.
	// We used to ignore it and download our own version but this makes it easier
	// to test out newer versions of Terraform.
	if installedVersion.GreaterThanOrEqual(maxTerraformVersion) {
		logger.Warn(ctx, "installed terraform version newer than expected, you may experience bugs",
			slog.F("installed_version", installedVersion.String()),
			slog.F("max_version", maxTerraformVersion.String()))
	}

	return absoluteBinary, nil
}

// Serve starts a dRPC server on the provided transport speaking Terraform provisioner.
func Serve(ctx context.Context, options *ServeOptions) error {
	if options.BinaryPath == "" {
		absoluteBinary, err := absoluteBinaryPath(ctx, options.Logger)
		if err != nil {
			// This is an early exit to prevent extra execution in case the context is canceled.
			// It generally happens in unit tests since this method is asynchronous and
			// the unit test kills the app before this is complete.
			if xerrors.Is(err, context.Canceled) {
				return xerrors.Errorf("absolute binary context canceled: %w", err)
			}

			options.Logger.Warn(ctx, "no usable terraform binary found, downloading to cache dir",
				slog.F("terraform_version", TerraformVersion.String()),
				slog.F("cache_dir", options.CachePath))
			binPath, err := Install(ctx, options.Logger, options.CachePath, TerraformVersion)
			if err != nil {
				return xerrors.Errorf("install terraform: %w", err)
			}
			options.BinaryPath = binPath
		} else {
			options.BinaryPath = absoluteBinary
		}
	}
	if options.Tracer == nil {
		options.Tracer = trace.NewNoopTracerProvider().Tracer("noop")
	}
	if options.ExitTimeout == 0 {
		options.ExitTimeout = unhanger.HungJobExitTimeout
	}
	return provisionersdk.Serve(ctx, &server{
		execMut:     &sync.Mutex{},
		binaryPath:  options.BinaryPath,
		cachePath:   options.CachePath,
		logger:      options.Logger,
		tracer:      options.Tracer,
		exitTimeout: options.ExitTimeout,
	}, options.ServeOptions)
}

type server struct {
	execMut     *sync.Mutex
	binaryPath  string
	cachePath   string
	logger      slog.Logger
	tracer      trace.Tracer
	exitTimeout time.Duration
}

func (s *server) startTrace(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return s.tracer.Start(ctx, name, append(opts, trace.WithAttributes(
		semconv.ServiceNameKey.String("coderd.provisionerd.terraform"),
	))...)
}

func (s *server) executor(workdir string) *executor {
	return &executor{
		server:     s,
		mut:        s.execMut,
		binaryPath: s.binaryPath,
		cachePath:  s.cachePath,
		workdir:    workdir,
		logger:     s.logger.Named("executor"),
	}
}
