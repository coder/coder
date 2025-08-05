package coderd_test

import (
	"archive/tar"
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/archive"
	"github.com/coder/coder/v2/archive/archivetest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPostFiles(t *testing.T) {
	t.Parallel()
	t.Run("BadContentType", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.Upload(ctx, "bad", bytes.NewReader([]byte{'a'}))
		require.Error(t, err)
	})

	t.Run("Insert", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(make([]byte, 1024)))
		require.NoError(t, err)
	})

	t.Run("InsertWindowsZip", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.Upload(ctx, "application/x-zip-compressed", bytes.NewReader(archivetest.TestZipFileBytes()))
		require.NoError(t, err)
	})

	t.Run("InsertAlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		data := make([]byte, 1024)
		_, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(data))
		require.NoError(t, err)
		_, err = client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(data))
		require.NoError(t, err)
	})
}

func TestDownload(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, _, err := client.Download(ctx, uuid.New())
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("InsertTar_DownloadTar", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		// given
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		tarball := archivetest.TestTarFileBytes()

		// when
		resp, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(tarball))
		require.NoError(t, err)
		data, contentType, err := client.Download(ctx, resp.ID)
		require.NoError(t, err)

		// then
		require.Len(t, data, len(tarball))
		require.Equal(t, codersdk.ContentTypeTar, contentType)
		require.Equal(t, tarball, data)
		archivetest.AssertSampleTarFile(t, data)
	})

	t.Run("InsertZip_DownloadTar", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		// given
		zipContent := archivetest.TestZipFileBytes()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// when
		resp, err := client.Upload(ctx, codersdk.ContentTypeZip, bytes.NewReader(zipContent))
		require.NoError(t, err)
		data, contentType, err := client.Download(ctx, resp.ID)
		require.NoError(t, err)

		// then
		require.Equal(t, codersdk.ContentTypeTar, contentType)

		// Note: creating a zip from a tar will result in some loss of information
		// as zip files do not store UNIX user:group data.
		archivetest.AssertSampleTarFile(t, data)
	})

	t.Run("InsertTar_DownloadZip", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		// given
		tarball := archivetest.TestTarFileBytes()

		tarReader := tar.NewReader(bytes.NewReader(tarball))
		expectedZip, err := archive.CreateZipFromTar(tarReader, 10240)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// when
		resp, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(tarball))
		require.NoError(t, err)
		data, contentType, err := client.DownloadWithFormat(ctx, resp.ID, codersdk.FormatZip)
		require.NoError(t, err)

		// then
		require.Equal(t, codersdk.ContentTypeZip, contentType)
		require.Equal(t, expectedZip, data)
		archivetest.AssertSampleZipFile(t, data)
	})
}
