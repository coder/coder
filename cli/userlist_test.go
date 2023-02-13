package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty/ptytest"
)

func TestUserList(t *testing.T) {
	t.Parallel()
	t.Run("Table", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		cmd, root := clitest.New(t, "users", "list")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		errC := make(chan error)
		go func() {
			errC <- cmd.Execute()
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch("coder.com")
	})
	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		cmd, root := clitest.New(t, "users", "list", "-o", "json")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})

		buf := bytes.NewBuffer(nil)
		cmd.SetOut(buf)
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			assert.NoError(t, err)
		}()

		<-doneChan

		var users []codersdk.User
		err := json.Unmarshal(buf.Bytes(), &users)
		require.NoError(t, err, "unmarshal JSON output")
		require.Len(t, users, 1)
		require.Contains(t, users[0].Email, "coder.com")
	})
	t.Run("NoURLFileErrorHasHelperText", func(t *testing.T) {
		t.Parallel()

		cmd, _ := clitest.New(t, "users", "list")

		_, err := cmd.ExecuteC()

		require.Contains(t, err.Error(), "Try logging in using 'coder login <url>'.")
	})
	t.Run("SessionAuthErrorHasHelperText", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		cmd, root := clitest.New(t, "users", "list")
		clitest.SetupConfig(t, client, root)

		_, err := cmd.ExecuteC()

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Contains(t, err.Error(), "Try logging in using 'coder login <url>'.")
	})
}

func TestUserShow(t *testing.T) {
	t.Parallel()

	t.Run("Table", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)
		_, otherUser := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		cmd, root := clitest.New(t, "users", "show", otherUser.Username)
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			assert.NoError(t, err)
		}()
		pty.ExpectMatch(otherUser.Email)
		<-doneChan
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		client := coderdtest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)
		other, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		otherUser, err := other.User(ctx, codersdk.Me)
		require.NoError(t, err, "fetch other user")
		cmd, root := clitest.New(t, "users", "show", otherUser.Username, "-o", "json")
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})

		buf := bytes.NewBuffer(nil)
		cmd.SetOut(buf)
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			assert.NoError(t, err)
		}()

		<-doneChan

		var newUser codersdk.User
		err = json.Unmarshal(buf.Bytes(), &newUser)
		require.NoError(t, err, "unmarshal JSON output")
		require.Equal(t, otherUser.ID, newUser.ID)
		require.Equal(t, otherUser.Username, newUser.Username)
		require.Equal(t, otherUser.Email, newUser.Email)
	})
}
