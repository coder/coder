package cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestUserCreate(t *testing.T) {
	t.Parallel()
	t.Run("Prompts", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		inv, root := clitest.New(t, "users", "create")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()
		matches := []string{
			"Username", "dean",
			"Email", "dean@coder.com",
			"Full name (optional):", "Mr. Dean Deanington",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		_ = testutil.RequireReceive(ctx, t, doneChan)
		created, err := client.User(ctx, matches[1])
		require.NoError(t, err)
		assert.Equal(t, matches[1], created.Username)
		assert.Equal(t, matches[3], created.Email)
		assert.Equal(t, matches[5], created.Name)
	})

	t.Run("PromptsNoName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		inv, root := clitest.New(t, "users", "create")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()
		matches := []string{
			"Username", "noname",
			"Email", "noname@coder.com",
			"Full name (optional):", "",
		}
		for i := 0; i < len(matches); i += 2 {
			match := matches[i]
			value := matches[i+1]
			pty.ExpectMatch(match)
			pty.WriteLine(value)
		}
		_ = testutil.RequireReceive(ctx, t, doneChan)
		created, err := client.User(ctx, matches[1])
		require.NoError(t, err)
		assert.Equal(t, matches[1], created.Username)
		assert.Equal(t, matches[3], created.Email)
		assert.Empty(t, created.Name)
	})

	t.Run("Args", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		args := []string{
			"users", "create",
			"-e", "dean@coder.com",
			"-u", "dean",
			"-n", "Mr. Dean Deanington",
			"-p", "1n5ecureP4ssw0rd!",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
		err := inv.Run()
		require.NoError(t, err)
		ctx := testutil.Context(t, testutil.WaitShort)
		created, err := client.User(ctx, "dean")
		require.NoError(t, err)
		assert.Equal(t, args[3], created.Email)
		assert.Equal(t, args[5], created.Username)
		assert.Equal(t, args[7], created.Name)
	})

	t.Run("ArgsNoName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		args := []string{
			"users", "create",
			"-e", "dean@coder.com",
			"-u", "dean",
			"-p", "1n5ecureP4ssw0rd!",
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, client, root)
		err := inv.Run()
		require.NoError(t, err)
		ctx := testutil.Context(t, testutil.WaitShort)
		created, err := client.User(ctx, args[5])
		require.NoError(t, err)
		assert.Equal(t, args[3], created.Email)
		assert.Equal(t, args[5], created.Username)
		assert.Empty(t, created.Name)
	})
}
