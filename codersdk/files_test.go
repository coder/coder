package codersdk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
)

func TestUpload(t *testing.T) {
	t.Parallel()
	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.UploadFile(context.Background(), "wow", []byte{})
		require.Error(t, err)
	})
	t.Run("Upload", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateInitialUser(t, client)
		_, err := client.UploadFile(context.Background(), codersdk.ContentTypeTar, []byte{'a'})
		require.NoError(t, err)
	})
}
