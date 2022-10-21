package terraform

import (
	"context"
	"errors"
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
	TerraformVersion = version.Must(version.NewVersion("1.3.0"))

	minTerraformVersion = version.Must(version.NewVersion("1.1.0"))
	maxTerraformVersion = version.Must(version.NewVersion("1.3.0"))

	terraformMinorVersionMismatch = xerrors.New("Terraform binary minor version mismatch.")
)

// Install implements a thread-safe, idempotent Terraform Install
// operation.
func Install(ctx context.Context, log slog.Logger, dir string, version *version.Version) (string, error) {
	err := os.MkdirAll(dir, 0750)
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

	versionFilePath := filepath.Join(dir, "version")
	versionFileContents, err := os.ReadFile(versionFilePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", xerrors.Errorf("read file: %w", err)
	}

	binPath := filepath.Join(dir, product.Terraform.BinaryName())
	_, err = os.Stat(binPath)
	if err == nil && version.String() == string(versionFileContents) {
		return binPath, nil
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
		slog.F("dir", dir),
		slog.F("version", TerraformVersion),
	)

	path, err := installer.Install(ctx)
	if err != nil {
		return "", xerrors.Errorf("install: %w", err)
	}

	// nolint: gosec
	err = os.WriteFile(versionFilePath, []byte(version.String()), 0700)
	if err != nil {
		return "", xerrors.Errorf("write %s: %w", versionFilePath, err)
	}

	return path, nil
}
