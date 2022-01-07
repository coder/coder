package terraform

import (
	"context"
	"os/exec"

	"github.com/coder/coder/provisionersdk"
	"github.com/hashicorp/go-version"
	"golang.org/x/xerrors"
)

// Serve starts a dRPC server on the provided transport speaking Terraform provisioner.
func Serve(ctx context.Context, options *provisionersdk.ServeOptions) error {
	minimumVersion, err := version.NewSemver("0.13.0")
	if err != nil {
		return xerrors.New("parse minimum version")
	}
	binaryPath, err := exec.LookPath("terraform")
	if err != nil {
		return xerrors.Errorf("terraform binary not found: %w", err)
	}
	return provisionersdk.Serve(ctx, &terraform{
		binaryPath:     binaryPath,
		minimumVersion: minimumVersion,
	}, options)
}

type terraform struct {
	binaryPath     string
	minimumVersion *version.Version
}
