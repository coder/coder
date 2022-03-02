package terraform

import (
	"context"
	"os/exec"

	"github.com/hashicorp/go-version"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

var (
	// The minimum version of Terraform supported by the provisioner.
	// Validation came out in 0.13.0, which was released August 10th, 2020.
	// https://www.hashicorp.com/blog/announcing-hashicorp-terraform-0-13
	minimumTerraformVersion = func() *version.Version {
		v, err := version.NewSemver("0.13.0")
		if err != nil {
			panic(err)
		}
		return v
	}()
)

type ServeOptions struct {
	*provisionersdk.ServeOptions

	// BinaryPath specifies the "terraform" binary to use.
	// If omitted, the $PATH will attempt to find it.
	BinaryPath string
	Logger     slog.Logger
}

// Serve starts a dRPC server on the provided transport speaking Terraform provisioner.
func Serve(ctx context.Context, options *ServeOptions) error {
	if options.BinaryPath == "" {
		binaryPath, err := exec.LookPath("terraform")
		if err != nil {
			return xerrors.Errorf("terraform binary not found: %w", err)
		}
		options.BinaryPath = binaryPath
	}
	shutdownCtx, shutdownCancel := context.WithCancel(ctx)
	return provisionersdk.Serve(ctx, &terraform{
		binaryPath:     options.BinaryPath,
		logger:         options.Logger,
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
	}, options.ServeOptions)
}

type terraform struct {
	binaryPath string
	logger     slog.Logger

	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

// Shutdown signals to begin graceful shutdown of any running operations.
func (t *terraform) Shutdown(_ context.Context, _ *proto.Empty) (*proto.Empty, error) {
	t.shutdownCancel()
	return &proto.Empty{}, nil
}
