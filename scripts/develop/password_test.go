//go:build !windows

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestLoadOrCreatePassword(t *testing.T) {
	t.Parallel()

	t.Run("GeneratesOnceAndReuses", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "nested", adminPasswordFile)

		first, err := loadOrCreatePassword(path)
		require.NoError(t, err)
		require.NoError(t, userpassword.Validate(first), "the server must accept a generated password")

		fi, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, first+"\n", string(raw))

		second, err := loadOrCreatePassword(path)
		require.NoError(t, err)
		assert.Equal(t, first, second)

		other, err := loadOrCreatePassword(filepath.Join(t.TempDir(), memberPasswordFile))
		require.NoError(t, err)
		assert.NotEqual(t, first, other)
	})

	t.Run("EmptyFileIsNotReplaced", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), adminPasswordFile)
		require.NoError(t, os.WriteFile(path, []byte(" \n"), 0o600))

		_, err := loadOrCreatePassword(path)
		require.ErrorContains(t, err, "is empty")
		require.ErrorContains(t, err, path)

		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, " \n", string(raw))
	})
}

func TestCheckPasswordFiles(t *testing.T) {
	t.Parallel()

	newConfig := func(t *testing.T) *devConfig {
		cfg := &devConfig{configDir: t.TempDir()}
		require.NoError(t, os.WriteFile(cfg.passwordPath(adminPasswordFile), nil, 0o600))
		return cfg
	}

	t.Run("EmptyAdminFile", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)
		require.ErrorContains(t, cfg.checkPasswordFiles(), adminPasswordFile)
	})

	t.Run("AdminFileIgnoredWithOverride", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)
		cfg.password = "an-override-password"
		require.NoError(t, cfg.checkPasswordFiles())
	})

	t.Run("NothingReadWhenSetupIsSkipped", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig(t)
		cfg.skipSetup = true
		require.NoError(t, cfg.checkPasswordFiles())
	})
}

func TestLoginBannerLines(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     devConfig
		want    []string
		notWant []string
	}{
		{
			name: "SetupSkipped",
		},
		{
			name: "Generated",
			cfg: devConfig{
				adminLogin:  devLogin{password: "admin-generated", seeded: true},
				memberLogin: devLogin{password: "member-generated", seeded: true},
			},
			want: []string{
				"admin@coder.com   admin-generated",
				"member@coder.com  member-generated",
				".coderv2/admin-password",
				".coderv2/member-password",
			},
			notWant: []string{"--password"},
		},
		{
			name: "Override",
			cfg: devConfig{
				adminLogin:  devLogin{password: "admin-override", fromFlag: true, seeded: true},
				memberLogin: devLogin{password: "member-generated", seeded: true},
			},
			want: []string{
				"admin@coder.com   admin-override",
				"(from --password)",
				".coderv2/member-password",
			},
			notWant: []string{".coderv2/admin-password"},
		},
		{
			name: "PasswordsNotSetByDevelop",
			cfg: devConfig{
				adminLogin:  devLogin{seeded: true},
				memberLogin: devLogin{seeded: true},
			},
			want: []string{
				"admin@coder.com   password not set by develop.sh",
				"member@coder.com  password not set by develop.sh",
			},
			notWant: []string{"Password files:"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lines := loginBannerLines(&tc.cfg)
			if len(tc.want) == 0 {
				require.Empty(t, lines)
				return
			}
			got := strings.Join(lines, "\n")
			for _, want := range tc.want {
				assert.Contains(t, got, want)
			}
			for _, notWant := range tc.notWant {
				assert.NotContains(t, got, notWant)
			}
			for _, line := range lines {
				assert.LessOrEqual(t, len(line), 64, "line must fit in the banner box: %q", line)
			}
		})
	}
}

func TestSetupFirstUser(t *testing.T) {
	t.Parallel()

	signIn := func(t *testing.T, client *codersdk.Client, email, password string) error {
		_, err := codersdk.New(client.URL).LoginWithPassword(testutil.Context(t, testutil.WaitShort), codersdk.LoginWithPasswordRequest{
			Email:    email,
			Password: password,
		})
		return err
	}
	requireUnauthorized := func(t *testing.T, err error) {
		apiErr, ok := codersdk.AsError(err)
		require.True(t, ok, "expected an API error, got %v", err)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	}

	t.Run("GeneratesStoresAndReuses", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := testutil.Logger(t)
		client := coderdtest.New(t, nil)
		cfg := &devConfig{configDir: t.TempDir()}

		_, err := setupFirstUser(ctx, logger, cfg, client.URL.String())
		require.NoError(t, err)

		admin, ok, err := readPassword(cfg.passwordPath(adminPasswordFile))
		require.NoError(t, err)
		require.True(t, ok)
		member, ok, err := readPassword(cfg.passwordPath(memberPasswordFile))
		require.NoError(t, err)
		require.True(t, ok)
		require.NotEqual(t, admin, member)
		require.Equal(t, devLogin{password: admin, seeded: true}, cfg.adminLogin)
		require.Equal(t, devLogin{password: member, seeded: true}, cfg.memberLogin)

		require.NoError(t, signIn(t, client, "member@coder.com", member))
		for _, email := range []string{"admin@coder.com", "member@coder.com"} {
			requireUnauthorized(t, signIn(t, client, email, oldDefaultPassword))
		}

		// Without a saved session the second run has to sign in with
		// the stored password.
		require.NoError(t, os.Remove(filepath.Join(cfg.configDir, "session")))
		again := &devConfig{configDir: cfg.configDir}
		_, err = setupFirstUser(ctx, logger, again, client.URL.String())
		require.NoError(t, err)
		require.Equal(t, cfg.adminLogin, again.adminLogin)
		require.Equal(t, cfg.memberLogin, again.memberLogin)
	})

	t.Run("OverrideIsUsedAndNotStored", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := coderdtest.New(t, nil)
		cfg := &devConfig{configDir: t.TempDir(), password: "an-override-password-1"}

		_, err := setupFirstUser(ctx, testutil.Logger(t), cfg, client.URL.String())
		require.NoError(t, err)

		require.NoFileExists(t, cfg.passwordPath(adminPasswordFile))
		require.NoError(t, signIn(t, client, "admin@coder.com", cfg.password))
		require.True(t, cfg.adminLogin.fromFlag)
		// The override is for the admin only.
		requireUnauthorized(t, signIn(t, client, "member@coder.com", cfg.password))
		require.NoError(t, signIn(t, client, "member@coder.com", cfg.memberLogin.password))
	})

	t.Run("UsersCreatedElsewhereKeepTheirPasswords", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := testutil.Logger(t)
		client := coderdtest.New(t, nil)
		const elsewhere = "a-password-set-elsewhere-1"

		// The first run uses an override, so no admin file is written.
		// It stands in for an instance whose users predate the files.
		first := &devConfig{configDir: t.TempDir(), password: elsewhere}
		_, err := setupFirstUser(ctx, logger, first, client.URL.String())
		require.NoError(t, err)
		require.NoError(t, os.Remove(first.passwordPath(memberPasswordFile)))

		// With a saved session develop carries on and writes no file.
		cfg := &devConfig{configDir: first.configDir}
		_, err = setupFirstUser(ctx, logger, cfg, client.URL.String())
		require.NoError(t, err)
		require.Equal(t, devLogin{seeded: true}, cfg.adminLogin)
		require.Equal(t, devLogin{seeded: true}, cfg.memberLogin)
		require.NoFileExists(t, cfg.passwordPath(adminPasswordFile))
		require.NoFileExists(t, cfg.passwordPath(memberPasswordFile))
		require.NoError(t, signIn(t, client, "admin@coder.com", elsewhere))

		// Without a session develop has no password to try.
		require.NoError(t, os.Remove(filepath.Join(cfg.configDir, "session")))
		_, err = setupFirstUser(ctx, logger, &devConfig{configDir: cfg.configDir}, client.URL.String())
		require.ErrorIs(t, err, errUnknownAdminPassword)
		require.NoFileExists(t, cfg.passwordPath(adminPasswordFile))

		// A stored password that the server refuses gives the same error.
		require.NoError(t, os.WriteFile(cfg.passwordPath(adminPasswordFile), []byte("not-the-right-password-1\n"), 0o600))
		_, err = setupFirstUser(ctx, logger, &devConfig{configDir: cfg.configDir}, client.URL.String())
		require.ErrorIs(t, err, errUnknownAdminPassword)
	})
}
