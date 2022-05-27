package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
)

func TestPostFiles(t *testing.T) {
	t.Parallel()
	t.Run("BadContentType", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		_, err := client.Upload(context.Background(), "bad", []byte{'a'})
		require.Error(t, err)
	})

	t.Run("Insert", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		_, err := client.Upload(context.Background(), codersdk.ContentTypeTar, make([]byte, 1024))
		require.NoError(t, err)
	})

	t.Run("InsertAlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		data := make([]byte, 1024)
		_, err := client.Upload(context.Background(), codersdk.ContentTypeTar, data)
		require.NoError(t, err)
		_, err = client.Upload(context.Background(), codersdk.ContentTypeTar, data)
		require.NoError(t, err)
	})
}

func TestDownload(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		_, _, err := client.Download(context.Background(), "something")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
	})

	t.Run("Insert", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		resp, err := client.Upload(context.Background(), codersdk.ContentTypeTar, make([]byte, 1024))
		require.NoError(t, err)
		data, contentType, err := client.Download(context.Background(), resp.Hash)
		require.NoError(t, err)
		require.Len(t, data, 1024)
		require.Equal(t, codersdk.ContentTypeTar, contentType)
	})
}
