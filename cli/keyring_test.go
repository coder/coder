package cli_test

import (
	"bytes"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/cli/sessionstore"
	"github.com/coder/coder/v2/cli/sessionstore/testhelpers"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

//nolint:paralleltest,tparallel // Windows OS keyring has intermittent failures with concurrent access
func TestUseKeyring(t *testing.T) {
	keyringSupported := runtime.GOOS == "windows" || runtime.GOOS == "darwin"
	client := coderdtest.New(t, nil)
	coderdtest.CreateFirstUser(t, client)

	// clitest always sets a custom --global-config, so an unset --use-keyring
	// selects file storage, and an explicit one selects the keyring.
	for _, tc := range []struct {
		name        string
		flags       []string
		env         string
		wantKeyring bool
	}{
		{name: "Unset"},
		{name: "Flag", flags: []string{"--use-keyring"}, wantKeyring: keyringSupported},
		{name: "Env", env: "true", wantKeyring: keyringSupported},
		{name: "FlagFalse", flags: []string{"--use-keyring=false"}},
		{name: "EnvFalse", env: "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serviceName := testhelpers.KeyringServiceName(t)
			keyring := sessionstore.NewKeyringWithService(serviceName)
			t.Cleanup(func() { _ = keyring.Delete(client.URL) })

			run := func(args ...string) (config.Root, string) {
				var root cli.RootCmd
				cmd, err := root.Command(root.AGPL())
				require.NoError(t, err)
				root.WithKeyringServiceName(serviceName)
				inv, cfg := clitest.NewWithCommand(t, cmd, append(args, tc.flags...)...)
				if tc.env != "" {
					inv.Environ.Set("CODER_USE_KEYRING", tc.env)
				}
				var stdout bytes.Buffer
				inv.Stdout = &stdout
				require.NoError(t, inv.Run())
				return cfg, stdout.String()
			}
			// requireStored checks which backend holds a session token.
			requireStored := func(cfg config.Root, inKeyring, inFile bool) {
				requireExists := func(err error, want bool) {
					if want {
						require.NoError(t, err)
					} else {
						require.ErrorIs(t, err, os.ErrNotExist)
					}
				}
				_, err := cfg.Session().Read()
				requireExists(err, inFile)
				if keyringSupported {
					_, err = keyring.Read(client.URL)
					requireExists(err, inKeyring)
				}
			}

			cfg, _ := run("login", "--token", client.SessionToken(), client.URL.String())
			requireStored(cfg, tc.wantKeyring, !tc.wantKeyring)

			// The stored token is a new session that authenticates, and the CLI
			// reads it back from the same backend.
			read := cfg.Session().Read
			if tc.wantKeyring {
				read = func() (string, error) { return keyring.Read(client.URL) }
			}
			stored, err := read()
			require.NoError(t, err)
			_, err = codersdk.New(client.URL, codersdk.WithSessionToken(stored)).User(testutil.Context(t, testutil.WaitShort), codersdk.Me)
			require.NoError(t, err, "stored token should authenticate")
			_, printed := run("login", "token", "--global-config", string(cfg))
			require.Equal(t, stored, strings.TrimSpace(printed))

			run("logout", "--yes", "--global-config", string(cfg))
			requireStored(cfg, false, false)
		})
	}
}
