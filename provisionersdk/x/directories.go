package x

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
)

var _ tfpath.LayoutInterface = (*TerraformDirectory)(nil)

func SessionDir(parentDir, sessID string, config *proto.Config) TerraformDirectory {
	if config.TemplateId == "" || config.TemplateId == uuid.Nil.String() ||
		config.TemplateVersionId == "" || config.TemplateVersionId == uuid.Nil.String() {
		return EphemeralSessionDir(parentDir, sessID)
	}

	return TerraformDirectory{
		workDirectory: filepath.Join(parentDir, config.TemplateId, config.TemplateVersionId),
		sessionID:     sessID,
		ephemeral:     false,
	}
}

// EphemeralSessionDir returns the directory name with mandatory prefix. These
// directories are created for each provisioning session and are meant to be
// ephemeral.
func EphemeralSessionDir(parentDir, sessID string) TerraformDirectory {
	return TerraformDirectory{
		workDirectory: filepath.Join(parentDir, sessionDirPrefix+sessID),
		sessionID:     sessID,
		ephemeral:     true,
	}
}

type TerraformDirectory struct {
	workDirectory string
	sessionID     string
	ephemeral     bool
}

const (
	// ReadmeFile is the location we look for to extract documentation from template versions.
	ReadmeFile = "README.md"

	sessionDirPrefix = "Session"
)

func (td TerraformDirectory) Cleanup(ctx context.Context, logger slog.Logger, fs afero.Fs) {
	var err error
	path := td.WorkDirectory()
	if !td.ephemeral {
		// Non-ephemeral directories only clean up the session subdirectory.
		// Leaving in place the wider work directory for reuse.
		path = td.StateSessionDirectory()
	}
	for attempt := 0; attempt < 5; attempt++ {
		err := fs.RemoveAll(path)
		if err != nil {
			// On Windows, open files cannot be removed.
			// When the provisioner daemon is shutting down,
			// it may take a few milliseconds for processes to exit.
			// See: https://github.com/golang/go/issues/50510
			logger.Debug(ctx, "failed to clean work directory; trying again", slog.Error(err))
			// TODO: Should we abort earlier if the context is done?
			time.Sleep(250 * time.Millisecond)
			continue
		}
		logger.Debug(ctx, "cleaned up work directory")
		return
	}

	logger.Error(ctx, "failed to clean up work directory after multiple attempts",
		slog.F("path", path), slog.Error(err))

	return
}

func (td TerraformDirectory) WorkDirectory() string {
	return td.workDirectory
}

// StateSessionDirectory follows the same directory structure as Terraform
// workspaces. All build specific state is stored within this directory.
//
// These files should be cleaned up on exit. In the case of a failure, they will
// not collide with other builds since each build uses a unique session ID.
func (td TerraformDirectory) StateSessionDirectory() string {
	return filepath.Join(td.workDirectory, "terraform.tfstate.d", td.sessionID)
}

func (td TerraformDirectory) StateFilePath() string {
	return filepath.Join(td.StateSessionDirectory(), "terraform.tfstate")
}

func (td TerraformDirectory) PlanFilePath() string {
	return filepath.Join(td.StateSessionDirectory(), "terraform.tfplan")
}

func (td TerraformDirectory) TerraformLockFile() string {
	return filepath.Join(td.WorkDirectory(), ".terraform.lock.hcl")
}

func (td TerraformDirectory) ReadmeFilePath() string {
	return filepath.Join(td.WorkDirectory(), ReadmeFile)
}

func (td TerraformDirectory) TerraformMetadataDir() string {
	return filepath.Join(td.WorkDirectory(), ".terraform")
}

func (td TerraformDirectory) ModulesDirectory() string {
	return filepath.Join(td.TerraformMetadataDir(), "modules")
}

func (td TerraformDirectory) ModulesFilePath() string {
	return filepath.Join(td.ModulesDirectory(), "modules.json")
}

func (td TerraformDirectory) ExtractArchive(ctx context.Context, logger slog.Logger, fs afero.Fs, cfg *proto.Config) error {
	logger.Info(ctx, "unpacking template source archive",
		slog.F("size_bytes", len(cfg.TemplateSourceArchive)),
	)

	err := fs.MkdirAll(td.WorkDirectory(), 0o700)
	if err != nil {
		return xerrors.Errorf("create work directory %q: %w", td.WorkDirectory(), err)
	}

	reader := tar.NewReader(bytes.NewBuffer(cfg.TemplateSourceArchive))
	// for safety, nil out the reference on Config, since the reader now owns it.
	cfg.TemplateSourceArchive = nil
	for {
		header, err := reader.Next()
		if err != nil {
			if xerrors.Is(err, io.EOF) {
				break
			}
			return xerrors.Errorf("read template source archive: %w", err)
		}
		logger.Debug(context.Background(), "read archive entry",
			slog.F("name", header.Name),
			slog.F("mod_time", header.ModTime),
			slog.F("size", header.Size))

		// Security: don't untar absolute or relative paths, as this can allow a malicious tar to overwrite
		// files outside the workdir.
		if !filepath.IsLocal(header.Name) {
			return xerrors.Errorf("refusing to extract to non-local path")
		}
		// nolint: gosec
		headerPath := filepath.Join(td.WorkDirectory(), header.Name)
		if !strings.HasPrefix(headerPath, filepath.Clean(td.WorkDirectory())) {
			return xerrors.New("tar attempts to target relative upper directory")
		}
		mode := header.FileInfo().Mode()
		if mode == 0 {
			mode = 0o600
		}

		// Always check for context cancellation before reading the next header.
		// This is mainly important for unit tests, since a canceled context means
		// the underlying directory is going to be deleted. There still exists
		// the small race condition that the context is canceled after this, and
		// before the disk write.
		if ctx.Err() != nil {
			return xerrors.Errorf("context canceled: %w", ctx.Err())
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = fs.MkdirAll(headerPath, mode)
			if err != nil {
				return xerrors.Errorf("mkdir %q: %w", headerPath, err)
			}
			logger.Debug(context.Background(), "extracted directory",
				slog.F("path", headerPath),
				slog.F("mode", fmt.Sprintf("%O", mode)))
		case tar.TypeReg:
			// TODO: If we are overwriting an existing file, that means we are reusing
			//  the terraform directory. In that case, we should check the file content
			//  matches what already exists on disk.
			file, err := fs.OpenFile(headerPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, mode)
			if err != nil {
				return xerrors.Errorf("create file %q (mode %s): %w", headerPath, mode, err)
			}

			hash := crc32.NewIEEE()
			hashReader := io.TeeReader(reader, hash)
			// Max file size of 10MiB.
			size, err := io.CopyN(file, hashReader, 10<<20)
			if xerrors.Is(err, io.EOF) {
				err = nil
			}
			if err != nil {
				_ = file.Close()
				return xerrors.Errorf("copy file %q: %w", headerPath, err)
			}
			err = file.Close()
			if err != nil {
				return xerrors.Errorf("close file %q: %s", headerPath, err)
			}
			logger.Debug(context.Background(), "extracted file",
				slog.F("size_bytes", size),
				slog.F("path", headerPath),
				slog.F("mode", mode),
				slog.F("checksum", fmt.Sprintf("%x", hash.Sum(nil))))
		}
	}

	return nil
}

// CleanInactiveTemplateVersions assumes this TerraformDirectory is the latest
// active template version. Assuming that, any other template version directories
// found alongside it are considered inactive and can be removed. Inactive
// template versions should use ephemeral TerraformDirectories.
func (td TerraformDirectory) CleanInactiveTemplateVersions(ctx context.Context, logger slog.Logger, fs afero.Fs) error {
	if td.ephemeral {
		return nil
	}

	wd := td.WorkDirectory()
	templateDir := filepath.Dir(wd)
	versionDir := filepath.Base(wd)

	entries, err := afero.ReadDir(fs, templateDir)
	if err != nil {
		return xerrors.Errorf("can't read %q directory: %w", templateDir, err)
	}

	for _, fi := range entries {
		if fi.IsDir() && fi.Name() == versionDir {
			continue
		}

		oldVerDir := filepath.Join(wd, fi.Name())
		logger.Info(ctx, "remove inactive template version directory", slog.F("version_path", oldVerDir))
		err = fs.RemoveAll(oldVerDir)
		if err != nil {
			return xerrors.Errorf("can't remove inactive template version %q: %w", fi.Name(), err)
		}
	}
	return nil
}
