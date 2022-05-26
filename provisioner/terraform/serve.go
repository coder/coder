package terraform

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

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
const terraformVersion = "1.1.9"
const versionDelimiter = "."

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

// Serve starts a dRPC server on the provided transport speaking Terraform provisioner.
func Serve(ctx context.Context, options *ServeOptions) error {
	if options.BinaryPath == "" {
		binaryPath, err := safeexec.LookPath("terraform")
		var downloadTerraform bool
		if err != nil {
			downloadTerraform = true
		} else {
			// If the "coder" binary is in the same directory as
			// the "terraform" binary, "terraform" is returned.
			//
			// We must resolve the absolute path for other processes
			// to execute this properly!
			absoluteBinary, err := filepath.Abs(binaryPath)
			if err != nil {
				return xerrors.Errorf("absolute: %w", err)
			}
			// Checking the installed version of Terraform.
			output, err := exec.Command(absoluteBinary, "version").Output()
			if err != nil {
				return xerrors.Errorf("terraform version: %w", err)
			}
			// The output for `terraform version` is:
			// Terraform v1.2.1
			// on linux_amd64
			versionRegex := regexp.MustCompile("Terraform v(.+)\n?.*")
			match := versionRegex.FindStringSubmatch(string(output))
			if match != nil {
				// match[0] is the entire string.
				// match[1] is the matched substring.
				version := match[1]
				terraformMinorVersion := strings.Join(strings.Split(terraformVersion, versionDelimiter)[:2], versionDelimiter)
				if !strings.HasPrefix(version, terraformMinorVersion) {
					downloadTerraform = true
				}
			} else {
				// Download the required Terraform version when unable to determine the existing one.
				downloadTerraform = true
			}
			if !downloadTerraform {
				options.BinaryPath = absoluteBinary
			}
		}
		if downloadTerraform {
			installer := &releases.ExactVersion{
				InstallDir: options.CachePath,
				Product:    product.Terraform,
				Version:    version.Must(version.NewVersion(terraformVersion)),
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
