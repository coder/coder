package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
)

func TestUserList(t *testing.T) {
	t.Parallel()
	t.Run("Table", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleUserAdmin())
		inv, root := clitest.New(t, "users", "list")
		clitest.SetupConfig(t, userAdmin, root)
		pty := ptytest.New(t).Attach(inv)
		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch("coder.com")
	})
	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleUserAdmin())
		inv, root := clitest.New(t, "users", "list", "-o", "json")
		clitest.SetupConfig(t, userAdmin, root)
		doneChan := make(chan struct{})

		buf := bytes.NewBuffer(nil)
		inv.Stdout = buf
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()

		<-doneChan

		var users []codersdk.User
		err := json.Unmarshal(buf.Bytes(), &users)
		require.NoError(t, err, "unmarshal JSON output")
		require.Len(t, users, 2)
		require.Contains(t, users[0].Email, "coder.com")
	})
	t.Run("NoURLFileErrorHasHelperText", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "users", "list")
		err := inv.Run()
		require.Contains(t, err.Error(), "Try logging in using 'coder login <url>'.")
	})
	t.Run("SessionAuthErrorHasHelperText", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		inv, root := clitest.New(t, "users", "list")
		clitest.SetupConfig(t, client, root)

		err := inv.Run()

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
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleUserAdmin())
		_, otherUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		inv, root := clitest.New(t, "users", "show", otherUser.Username)
		clitest.SetupConfig(t, userAdmin, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()
		pty.ExpectMatch(otherUser.Email)
		<-doneChan
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleUserAdmin())
		other, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		otherUser, err := other.User(ctx, codersdk.Me)
		require.NoError(t, err, "fetch other user")
		inv, root := clitest.New(t, "users", "show", otherUser.Username, "-o", "json")
		clitest.SetupConfig(t, userAdmin, root)
		doneChan := make(chan struct{})

		buf := bytes.NewBuffer(nil)
		inv.Stdout = buf
		go func() {
			defer close(doneChan)
			err := inv.Run()
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
