package coderd_test

import (
	"context"
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
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.UploadFile(context.Background(), "bad", []byte{'a'})
		require.Error(t, err)
	})

	t.Run("Insert", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.UploadFile(context.Background(), codersdk.ContentTypeTar, make([]byte, 1024))
		require.NoError(t, err)
	})

	t.Run("InsertAlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateInitialUser(t, client)
		data := make([]byte, 1024)
		_, err := client.UploadFile(context.Background(), codersdk.ContentTypeTar, data)
		require.NoError(t, err)
		_, err = client.UploadFile(context.Background(), codersdk.ContentTypeTar, data)
		require.NoError(t, err)
	})
}
