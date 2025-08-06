package coderd_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
)

func TestInitScript(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		script, err := client.InitScript(context.Background(), "windows", "amd64")
		require.NoError(t, err)
		require.NotEmpty(t, script)
	})

	t.Run("BadRequest", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_, err := client.InitScript(context.Background(), "darwin", "armv7")
		require.Error(t, err)
		fmt.Printf("err: %+v\n", err)
	})
}
