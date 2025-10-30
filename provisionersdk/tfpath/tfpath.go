package tfpath

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

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

const (
	// ReadmeFile is the location we look for to extract documentation from template versions.
	ReadmeFile = "README.md"

	sessionDirPrefix = "Session"
)

func Session(parent, sessionID string) Layout {
	return Layout(filepath.Join(parent, sessionDirPrefix+sessionID))
}

// TODO: Maybe we should include the afero.FS here as well, then all operations
// would be on the same FS?
type Layout string

// WorkDirectory returns the root working directory for Terraform files.
func (l Layout) WorkDirectory() string { return string(l) }

func (l Layout) StateFilePath() string {
	return filepath.Join(l.WorkDirectory(), "terraform.tfstate")
}

func (l Layout) PlanFilePath() string {
	return filepath.Join(l.WorkDirectory(), "terraform.tfplan")
}

func (l Layout) TerraformLockFile() string {
	return filepath.Join(l.WorkDirectory(), ".terraform.lock.hcl")
}

func (l Layout) ReadmeFilePath() string {
	return filepath.Join(l.WorkDirectory(), ReadmeFile)
}

func (l Layout) TerraformMetadataDir() string {
	return filepath.Join(l.WorkDirectory(), ".terraform")
}

func (l Layout) ModulesDirectory() string {
	return filepath.Join(l.TerraformMetadataDir(), "modules")
}

func (l Layout) ModulesFilePath() string {
	return filepath.Join(l.ModulesDirectory(), "modules.json")
}

func (l Layout) ExtractArchive(ctx context.Context, logger slog.Logger, fs afero.Fs, cfg *proto.Config) error {
	logger.Info(ctx, "unpacking template source archive",
		slog.F("size_bytes", len(cfg.TemplateSourceArchive)),
	)

	err := fs.MkdirAll(l.WorkDirectory(), 0o700)
	if err != nil {
		return xerrors.Errorf("create work directory %q: %w", l.WorkDirectory(), err)
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
		headerPath := filepath.Join(l.WorkDirectory(), header.Name)
		if !strings.HasPrefix(headerPath, filepath.Clean(l.WorkDirectory())) {
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
			file, err := fs.OpenFile(headerPath, os.O_CREATE|os.O_RDWR, mode)
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

// Cleanup removes the work directory and all of its contents.
func (l Layout) Cleanup(ctx context.Context, logger slog.Logger, fs afero.Fs) {
	var err error
	path := l.WorkDirectory()

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

	// Returning an error at this point cannot do any good. The caller cannot resolve
	// this. There is a routine cleanup task that will remove old work directories
	// when this fails.
	logger.Error(ctx, "failed to clean up work directory after multiple attempts",
		slog.F("path", path), slog.Error(err))
}
