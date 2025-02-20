package terraform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
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
	// NOTE: Keep this in sync with the version in install.sh.
	TerraformVersion = version.Must(version.NewVersion("1.10.5"))

	minTerraformVersion = version.Must(version.NewVersion("1.1.0"))
	maxTerraformVersion = version.Must(version.NewVersion("1.10.9")) // use .9 to automatically allow patch releases

	terraformMinorVersionMismatch = xerrors.New("Terraform binary minor version mismatch.")
)

// Install implements a thread-safe, idempotent Terraform Install
// operation.
//
//nolint:revive // verbose is a control flag that controls the verbosity of the log output.
func Install(ctx context.Context, log slog.Logger, verbose bool, dir string, wantVersion *version.Version) (string, error) {
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

	hasVersionStr := "nil"
	hasVersion, err := versionFromBinaryPath(ctx, binPath)
	if err == nil {
		hasVersionStr = hasVersion.String()
		if hasVersion.Equal(wantVersion) {
			return binPath, err
		}
	}

	installer := &releases.ExactVersion{
		InstallDir: dir,
		Product:    product.Terraform,
		Version:    TerraformVersion,
	}
	installer.SetLogger(slog.Stdlib(ctx, log, slog.LevelDebug))

	logInstall := log.Debug
	if verbose {
		logInstall = log.Info
	}

	logInstall(ctx, "installing terraform",
		slog.F("prev_version", hasVersionStr),
		slog.F("dir", dir),
		slog.F("version", TerraformVersion))

	prolongedInstall := atomic.Bool{}
	prolongedInstallCtx, prolongedInstallCancel := context.WithCancel(ctx)
	go func() {
		seconds := 15
		select {
		case <-time.After(time.Duration(seconds) * time.Second):
			prolongedInstall.Store(true)
			// We always want to log this at the info level.
			log.Info(
				prolongedInstallCtx,
				fmt.Sprintf("terraform installation is taking longer than %d seconds, still in progress", seconds),
				slog.F("prev_version", hasVersionStr),
				slog.F("dir", dir),
				slog.F("version", TerraformVersion),
			)
		case <-prolongedInstallCtx.Done():
			return
		}
	}()
	defer prolongedInstallCancel()

	path, err := installer.Install(ctx)
	if err != nil {
		return "", xerrors.Errorf("install: %w", err)
	}

	// Sanity-check: if path != binPath then future invocations of Install
	// will fail.
	if path != binPath {
		return "", xerrors.Errorf("%s should be %s", path, binPath)
	}

	if prolongedInstall.Load() {
		log.Info(ctx, "terraform installation complete")
	}

	return path, nil
}
