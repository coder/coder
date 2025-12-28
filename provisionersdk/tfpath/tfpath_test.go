package tfpath_test

import (
	"archive/tar"
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
	"github.com/coder/coder/v2/testutil"
)

func TestExtractArchive(t *testing.T) {
	t.Parallel()

	t.Run("EmptyFileOverwritesExistingContent", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// Create an in-memory filesystem
		fs := afero.NewMemMapFs()
		logger := testutil.Logger(t).Leveled(slog.LevelDebug)

		// Create a session directory
		sessDir := tfpath.Session("/tmp/test", uuid.NewString())

		// Pre-create a file with existing content
		err := fs.MkdirAll(sessDir.WorkDirectory(), 0o755)
		require.NoError(t, err)
		existingFilePath := sessDir.WorkDirectory() + "/preset.tf"
		err = afero.WriteFile(fs, existingFilePath, []byte("old preset content that should be replaced"), 0o644)
		require.NoError(t, err)

		// Verify the file has content
		content, err := afero.ReadFile(fs, existingFilePath)
		require.NoError(t, err)
		require.Equal(t, "old preset content that should be replaced", string(content))

		// Create a tar archive with an EMPTY file (simulating cleared file)
		var tarBuf bytes.Buffer
		tw := tar.NewWriter(&tarBuf)
		err = tw.WriteHeader(&tar.Header{
			Name:     "preset.tf",
			Size:     0, // Empty file
			Mode:     0o644,
			Typeflag: tar.TypeReg,
		})
		require.NoError(t, err)
		// No content written - file is empty
		require.NoError(t, tw.Close())

		// Extract the archive - should overwrite with empty content
		err = sessDir.ExtractArchive(ctx, logger, fs, tarBuf.Bytes())
		require.NoError(t, err)

		// Verify the file is now empty (not containing old content)
		content, err = afero.ReadFile(fs, existingFilePath)
		require.NoError(t, err)
		require.Empty(t, content, "file should be empty after extracting empty file from archive")
	})

	t.Run("NonEmptyFileExtractsCorrectly", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		fs := afero.NewMemMapFs()
		logger := testutil.Logger(t).Leveled(slog.LevelDebug)

		sessDir := tfpath.Session("/tmp/test", uuid.NewString())

		// Create a tar archive with a non-empty file
		var tarBuf bytes.Buffer
		tw := tar.NewWriter(&tarBuf)
		fileContent := []byte("terraform { required_version = \">= 1.0\" }")
		err := tw.WriteHeader(&tar.Header{
			Name:     "main.tf",
			Size:     int64(len(fileContent)),
			Mode:     0o644,
			Typeflag: tar.TypeReg,
		})
		require.NoError(t, err)
		_, err = tw.Write(fileContent)
		require.NoError(t, err)
		require.NoError(t, tw.Close())

		// Extract the archive
		err = sessDir.ExtractArchive(ctx, logger, fs, tarBuf.Bytes())
		require.NoError(t, err)

		// Verify the file has the correct content
		filePath := sessDir.WorkDirectory() + "/main.tf"
		content, err := afero.ReadFile(fs, filePath)
		require.NoError(t, err)
		require.Equal(t, fileContent, content)
	})

	t.Run("MixedEmptyAndNonEmptyFiles", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		fs := afero.NewMemMapFs()
		logger := testutil.Logger(t).Leveled(slog.LevelDebug)

		sessDir := tfpath.Session("/tmp/test", uuid.NewString())

		// Create a tar archive with both empty and non-empty files
		var tarBuf bytes.Buffer
		tw := tar.NewWriter(&tarBuf)

		// Empty file
		err := tw.WriteHeader(&tar.Header{
			Name:     "empty.tf",
			Size:     0,
			Mode:     0o644,
			Typeflag: tar.TypeReg,
		})
		require.NoError(t, err)

		// Non-empty file
		mainContent := []byte("resource \"null_resource\" \"test\" {}")
		err = tw.WriteHeader(&tar.Header{
			Name:     "main.tf",
			Size:     int64(len(mainContent)),
			Mode:     0o644,
			Typeflag: tar.TypeReg,
		})
		require.NoError(t, err)
		_, err = tw.Write(mainContent)
		require.NoError(t, err)

		require.NoError(t, tw.Close())

		// Extract
		err = sessDir.ExtractArchive(ctx, logger, fs, tarBuf.Bytes())
		require.NoError(t, err)

		// Verify empty file
		emptyPath := sessDir.WorkDirectory() + "/empty.tf"
		content, err := afero.ReadFile(fs, emptyPath)
		require.NoError(t, err)
		require.Empty(t, content)

		// Verify non-empty file
		mainPath := sessDir.WorkDirectory() + "/main.tf"
		content, err = afero.ReadFile(fs, mainPath)
		require.NoError(t, err)
		require.Equal(t, mainContent, content)
	})
}
