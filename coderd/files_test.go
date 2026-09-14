package coderd_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/archive"
	"github.com/coder/coder/v2/archive/archivetest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPostFiles(t *testing.T) {
	t.Parallel()

	buildZipWithFile := func(t *testing.T, name string, writeContents func(w io.Writer) error) []byte {
		t.Helper()

		var zipBytes bytes.Buffer
		zw := zip.NewWriter(&zipBytes)
		w, err := zw.Create(name)
		require.NoError(t, err)
		require.NoError(t, writeContents(w))
		require.NoError(t, zw.Close())

		return zipBytes.Bytes()
	}

	// Single instance shared across all sub-tests. Each sub-test
	// creates independent resources with unique IDs so parallel
	// execution is safe.
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	t.Run("BadContentType", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.Upload(ctx, "bad", bytes.NewReader([]byte{'a'}))
		require.Error(t, err)
	})

	t.Run("Insert", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(make([]byte, 1024)))
		require.NoError(t, err)
	})

	t.Run("InsertWindowsZip", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.Upload(ctx, "application/x-zip-compressed", bytes.NewReader(archivetest.TestZipFileBytes()))
		require.NoError(t, err)
	})

	t.Run("InsertAlreadyExists", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		data := make([]byte, 1024)
		_, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(data))
		require.NoError(t, err)
		_, err = client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(data))
		require.NoError(t, err)
	})
	t.Run("InvalidZipMetadata", func(t *testing.T) {
		t.Parallel()

		corruptZipUncompressedSize := func(t *testing.T, zipBytes []byte, size uint32) []byte {
			t.Helper()

			const (
				directoryHeaderSignature = "PK\x01\x02"
				uncompressedSizeOffset   = 24
			)
			hdrOffset := bytes.Index(zipBytes, []byte(directoryHeaderSignature))
			require.NotEqual(t, -1, hdrOffset, "missing ZIP central directory header")
			corrupted := bytes.Clone(zipBytes)
			sizeBytes := corrupted[hdrOffset+uncompressedSizeOffset : hdrOffset+uncompressedSizeOffset+4]
			binary.LittleEndian.PutUint32(sizeBytes, size)

			return corrupted
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		zipBytes := buildZipWithFile(t, "hello.txt", func(w io.Writer) error {
			_, err := w.Write([]byte("hello"))
			return err
		})
		zipBytes = corruptZipUncompressedSize(t, zipBytes, 6)

		_, err := client.Upload(ctx, codersdk.ContentTypeZip, bytes.NewReader(zipBytes))
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})
	t.Run("InsertConcurrent", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		var wg sync.WaitGroup
		var end sync.WaitGroup
		wg.Add(1)
		end.Add(3)
		for range 3 {
			go func() {
				wg.Wait()
				data := make([]byte, 1024)
				_, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(data))
				end.Done()
				assert.NoError(t, err)
			}()
		}
		wg.Done()
		end.Wait()
	})
	//nolint:paralleltest // This subtest is intentionally serial to
	// avoid extra memory pressure.
	t.Run("OversizedZipExpansion", func(t *testing.T) {
		buildZipWithSizedFile := func(t *testing.T, name string, size int64) []byte {
			return buildZipWithFile(t, name, func(w io.Writer) error {
				chunk := bytes.Repeat([]byte("a"), 32*1024)
				for written := int64(0); written < size; {
					n := len(chunk)
					if remaining := size - written; int64(n) > remaining {
						n = int(remaining)
					}

					_, err := w.Write(chunk[:n])
					if err != nil {
						return err
					}
					written += int64(n)
				}

				return nil
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Leave only enough room for the tar trailer. The single
		// entry header then pushes the converted tar output over the
		// file size limit.
		size := int64(coderd.HTTPFileMaxBytes - 1024)
		zipBytes := buildZipWithSizedFile(t, "oversized.txt", size)

		_, err := client.Upload(ctx, codersdk.ContentTypeZip, bytes.NewReader(zipBytes))
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusRequestEntityTooLarge, apiErr.StatusCode())
	})
}

func TestDownload(t *testing.T) {
	t.Parallel()

	// Shared instance — see TestPostFiles for rationale.
	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, _, err := client.Download(ctx, uuid.New())
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("InsertTar_DownloadTar", func(t *testing.T) {
		t.Parallel()
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
