package terraform

import (
	"context"
	"path/filepath"

	"github.com/cli/safeexec"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/provisionersdk"
)

// This is the exact version of Terraform used internally
// when Terraform is missing on the system.
var terraformVersion = version.Must(version.NewVersion("1.1.9"))
var minTerraformVersion = version.Must(version.NewVersion("1.1.0"))
var maxTerraformVersion = version.Must(version.NewVersion("1.2.0"))

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
	CachePath  string
	Logger     slog.Logger
}

func getAbsoluteBinaryPath(ctx context.Context) (string, bool) {
	binaryPath, err := safeexec.LookPath("terraform")
	if err != nil {
		return "", false
	}
	// If the "coder" binary is in the same directory as
	// the "terraform" binary, "terraform" is returned.
	//
	// We must resolve the absolute path for other processes
	// to execute this properly!
	absoluteBinary, err := filepath.Abs(binaryPath)
	if err != nil {
		return "", false
	}
	// Checking the installed version of Terraform.
	version, err := versionFromBinaryPath(ctx, absoluteBinary)
	if err != nil {
		return "", false
	} else if version.LessThan(minTerraformVersion) || version.GreaterThanOrEqual(maxTerraformVersion) {
		return "", false
	}
	return absoluteBinary, true
}

// Serve starts a dRPC server on the provided transport speaking Terraform provisioner.
func Serve(ctx context.Context, options *ServeOptions) error {
	if options.BinaryPath == "" {
		absoluteBinary, ok := getAbsoluteBinaryPath(ctx)

		if ok {
			options.BinaryPath = absoluteBinary
		} else {
			installer := &releases.ExactVersion{
				InstallDir: options.CachePath,
				Product:    product.Terraform,
				Version:    terraformVersion,
			}

			execPath, err := installer.Install(ctx)
			if err != nil {
				return xerrors.Errorf("install terraform: %w", err)
			}
			options.BinaryPath = execPath
		}
	}
	return provisionersdk.Serve(ctx, &server{
		binaryPath: options.BinaryPath,
		cachePath:  options.CachePath,
		logger:     options.Logger,
	}, options.ServeOptions)
}

type server struct {
	binaryPath string
	cachePath  string
	logger     slog.Logger
}

func (t server) executor(workdir string) executor {
	return executor{
		binaryPath: t.binaryPath,
		cachePath:  t.cachePath,
		workdir:    workdir,
	}
}
