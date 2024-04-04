package cli_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestAuthToken(t *testing.T) {
	t.Parallel()
	t.Run("ValidUser", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "auth", "token")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		split := strings.Split(client.SessionToken(), "-")
		loginKey, err := client.APIKeyByID(ctx, codersdk.Me, split[0])
		require.NoError(t, err)

		doneChan := make(chan struct{})
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		pty.ExpectMatch(fmt.Sprintf("Your session token '%s' expires at %s.", client.SessionToken(), loginKey.ExpiresAt))
		<-doneChan
	})

	t.Run("NoUser", func(t *testing.T) {
		t.Parallel()
		inv, _ := clitest.New(t, "auth", "token")

		err := inv.Run()
		errorMsg := "You are not logged in."
		assert.ErrorContains(t, err, errorMsg)
	})
}

func TestAuthStatus(t *testing.T) {
	t.Parallel()
	t.Run("ValidUser", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "auth", "status")
		clitest.SetupConfig(t, client, root)

		defaultUsername := "testuser"

		pty := ptytest.New(t).Attach(inv)
		doneChan := make(chan struct{})
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		pty.ExpectMatch(fmt.Sprintf("Hello there, %s! You're authenticated at %s.", defaultUsername, client.URL.String()))
		<-doneChan
	})

	t.Run("NoUser", func(t *testing.T) {
		t.Parallel()
		inv, _ := clitest.New(t, "auth", "status")

		err := inv.Run()
		errorMsg := "You are not logged in."
		assert.ErrorContains(t, err, errorMsg)
	})
}
