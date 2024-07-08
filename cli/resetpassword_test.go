package cli_test

import (
	"context"
	"net/url"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

// nolint:paralleltest
func TestResetPassword(t *testing.T) {
	// dbtestutil.Open() seems to be creating race conditions when run in parallel.
	// t.Parallel()

	if runtime.GOOS != "linux" || testing.Short() {
		// Skip on non-Linux because it spawns a PostgreSQL instance.
		t.SkipNow()
	}

	const email = "some@one.com"
	const username = "example"
	const oldPassword = "MyOldPassword!"
	const newPassword = "MyNewPassword!"

	// start postgres and coder server processes
	connectionURL, closeFunc, err := dbtestutil.Open()
	require.NoError(t, err)
	defer closeFunc()
	ctx, cancelFunc := context.WithCancel(context.Background())
	serverDone := make(chan struct{})
	serverinv, cfg := clitest.New(t,
		"server",
		"--http-address", ":0",
		"--access-url", "http://example.com",
		"--postgres-url", connectionURL,
		"--cache-dir", t.TempDir(),
	)
	go func() {
		defer close(serverDone)
		err = serverinv.WithContext(ctx).Run()
		assert.NoError(t, err)
	}()
	var rawURL string
	require.Eventually(t, func() bool {
		rawURL, err = cfg.URL().Read()
		return err == nil && rawURL != ""
	}, testutil.WaitLong, testutil.IntervalFast)
	accessURL, err := url.Parse(rawURL)
	require.NoError(t, err)
	client := codersdk.New(accessURL)

	_, err = client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
		Email:    email,
		Username: username,
		Password: oldPassword,
	})
	require.NoError(t, err)

	// reset the password

	resetinv, cmdCfg := clitest.New(t, "reset-password", "--postgres-url", connectionURL, username)
	clitest.SetupConfig(t, client, cmdCfg)
	cmdDone := make(chan struct{})
	pty := ptytest.New(t)
	resetinv.Stdin = pty.Input()
	resetinv.Stdout = pty.Output()
	go func() {
		defer close(cmdDone)
		err = resetinv.Run()
		assert.NoError(t, err)
	}()

	matches := []struct {
		output string
		input  string
	}{
		{"Enter new", newPassword},
		{"Confirm", newPassword},
	}
	for _, match := range matches {
		pty.ExpectMatch(match.output)
		pty.WriteLine(match.input)
	}
	<-cmdDone

	// now try logging in

	_, err = client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
		Email:    email,
		Password: oldPassword,
	})
	require.Error(t, err)

	_, err = client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
		Email:    email,
		Password: newPassword,
	})
	require.NoError(t, err)

	cancelFunc()
	<-serverDone
}
