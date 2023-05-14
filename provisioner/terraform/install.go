package terraform

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

var (
	// TerraformVersion is the version of Terraform used internally
	// when Terraform is not available on the system.
	// NOTE: Keep this in sync with the version in scripts/Dockerfile.base.
	TerraformVersion = version.Must(version.NewVersion("1.3.4"))

	minTerraformVersion = version.Must(version.NewVersion("1.1.0"))
	maxTerraformVersion = version.Must(version.NewVersion("1.3.9"))

	terraformMinorVersionMismatch = xerrors.New("Terraform binary minor version mismatch.")
)

// Install implements a thread-safe, idempotent Terraform Install
// operation.
func Install(ctx context.Context, log slog.Logger, dir string, wantVersion *version.Version) (string, error) {
	err := os.MkdirAll(dir, 0o750)
	if err != nil {
		return "", err
	}

	// Windows requires a separate lock file.
	// See https://github.com/pinterest/knox/blob/master/client/flock_windows.go#L64
	// for precedent.
	lockFilePath := filepath.Join(dir, "lock")
	lock := flock.New(lockFilePath)
	ok, err := lock.TryLockContext(ctx, time.Millisecond*100)
	if !ok {
		return "", xerrors.Errorf("could not acquire flock for %v: %w", lockFilePath, err)
	}
	defer lock.Close()

	binPath := filepath.Join(dir, product.Terraform.BinaryName())

	hasVersion, err := versionFromBinaryPath(ctx, binPath)
	if err == nil && hasVersion.Equal(wantVersion) {
		return binPath, err
	}

	installer := &releases.ExactVersion{
		InstallDir: dir,
		Product:    product.Terraform,
		Version:    TerraformVersion,
	}
	installer.SetLogger(slog.Stdlib(ctx, log, slog.LevelDebug))
	log.Debug(
		ctx,
		"installing terraform",
		slog.F("prev_version", hasVersion),
		slog.F("dir", dir),
		slog.F("version", TerraformVersion),
	)

	path, err := installer.Install(ctx)
	if err != nil {
		return "", xerrors.Errorf("install: %w", err)
	}

	// Sanity-check: if path != binPath then future invocations of Install
	// will fail.
	if path != binPath {
		return "", xerrors.Errorf("%s should be %s", path, binPath)
	}

	return path, nil
}
