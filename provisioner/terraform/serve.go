package terraform

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/cli/safeexec"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/provisionersdk"
)

var (
	// TerraformVersion is the version of Terraform used internally
	// when Terraform is not available on the system.
	TerraformVersion = version.Must(version.NewVersion("1.3.0"))

	minTerraformVersion = version.Must(version.NewVersion("1.1.0"))
	maxTerraformVersion = version.Must(version.NewVersion("1.3.0"))

	terraformMinorVersionMismatch = xerrors.New("Terraform binary minor version mismatch.")

	installTerraform         sync.Once
	installTerraformExecPath string
	// nolint:errname
	installTerraformError error
)

const (
	defaultExitTimeout = 5 * time.Minute
)

type ServeOptions struct {
	*provisionersdk.ServeOptions

	// BinaryPath specifies the "terraform" binary to use.
	// If omitted, the $PATH will attempt to find it.
	BinaryPath string
	CachePath  string
	Logger     slog.Logger

	// ExitTimeout defines how long we will wait for a running Terraform
	// command to exit (cleanly) if the provision was stopped. This only
	// happens when the command is still running after the provision
	// stream is closed. If the provision is canceled via RPC, this
	// timeout will not be used.
	//
	// This is a no-op on Windows where the process can't be interrupted.
	//
	// Default value: 5 minutes.
	ExitTimeout time.Duration
}

func absoluteBinaryPath(ctx context.Context) (string, error) {
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
	version, err := versionFromBinaryPath(ctx, absoluteBinary)
	if err != nil {
		return "", xerrors.Errorf("Terraform binary get version failed: %w", err)
	}

	if version.LessThan(minTerraformVersion) || version.GreaterThan(maxTerraformVersion) {
		return "", terraformMinorVersionMismatch
	}

	return absoluteBinary, nil
}

// Serve starts a dRPC server on the provided transport speaking Terraform provisioner.
func Serve(ctx context.Context, options *ServeOptions) error {
	if options.BinaryPath == "" {
		absoluteBinary, err := absoluteBinaryPath(ctx)
		if err != nil {
			// This is an early exit to prevent extra execution in case the context is canceled.
			// It generally happens in unit tests since this method is asynchronous and
			// the unit test kills the app before this is complete.
			if xerrors.Is(err, context.Canceled) {
				return xerrors.Errorf("absolute binary context canceled: %w", err)
			}

			// We don't want to install Terraform multiple times!
			installTerraform.Do(func() {
				installer := &releases.ExactVersion{
					InstallDir: options.CachePath,
					Product:    product.Terraform,
					Version:    TerraformVersion,
				}
				installer.SetLogger(slog.Stdlib(ctx, options.Logger, slog.LevelDebug))
				options.Logger.Debug(ctx, "installing terraform", slog.F("dir", options.CachePath), slog.F("version", TerraformVersion))
				installTerraformExecPath, installTerraformError = installer.Install(ctx)
			})
			if installTerraformError != nil {
				return xerrors.Errorf("install terraform: %w", installTerraformError)
			}
			options.BinaryPath = installTerraformExecPath
		} else {
			options.BinaryPath = absoluteBinary
		}
	}
	if options.ExitTimeout == 0 {
		options.ExitTimeout = defaultExitTimeout
	}
	return provisionersdk.Serve(ctx, &server{
		binaryPath:  options.BinaryPath,
		cachePath:   options.CachePath,
		logger:      options.Logger,
		exitTimeout: options.ExitTimeout,
	}, options.ServeOptions)
}

type server struct {
	// initMu protects against executors running `terraform init`
	// concurrently when cache path is set.
	initMu sync.Mutex

	binaryPath string
	cachePath  string
	logger     slog.Logger

	exitTimeout time.Duration
}

func (s *server) executor(workdir string) executor {
	return executor{
		initMu:     &s.initMu,
		binaryPath: s.binaryPath,
		cachePath:  s.cachePath,
		workdir:    workdir,
	}
}
