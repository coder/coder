//go:build !windows

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/config"
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

	t.Run("OverrideIsStoredWhenNoPasswordIs", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := coderdtest.New(t, nil)
		cfg := &devConfig{configDir: t.TempDir(), password: "an-override-password-1"}

		_, err := setupFirstUser(ctx, testutil.Logger(t), cfg, client.URL.String())
		require.NoError(t, err)

		require.NoError(t, signIn(t, client, "admin@coder.com", cfg.password))
		require.True(t, cfg.adminLogin.fromFlag)
		// The override is for the admin only.
		requireUnauthorized(t, signIn(t, client, "member@coder.com", cfg.password))
		require.NoError(t, signIn(t, client, "member@coder.com", cfg.memberLogin.password))

		stored, ok, err := readPassword(cfg.passwordPath(adminPasswordFile))
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, cfg.password, stored)

		// The next run needs neither the flag nor the saved session.
		require.NoError(t, os.Remove(filepath.Join(cfg.configDir, "session")))
		again := &devConfig{configDir: cfg.configDir}
		_, err = setupFirstUser(ctx, testutil.Logger(t), again, client.URL.String())
		require.NoError(t, err)
		require.Equal(t, devLogin{password: cfg.password, seeded: true}, again.adminLogin)
	})

	t.Run("OverrideDoesNotReplaceAStoredPassword", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client := coderdtest.New(t, nil)
		cfg := &devConfig{configDir: t.TempDir(), password: "an-override-password-1"}
		const stored = "a-password-stored-earlier-1"
		require.NoError(t, os.WriteFile(cfg.passwordPath(adminPasswordFile), []byte(stored+"\n"), 0o600))

		_, err := setupFirstUser(ctx, testutil.Logger(t), cfg, client.URL.String())
		require.NoError(t, err)
		require.NoError(t, signIn(t, client, "admin@coder.com", cfg.password))
		got, ok, err := readPassword(cfg.passwordPath(adminPasswordFile))
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, stored, got)
	})

	// oldInstance stands in for an instance from before passwords were
	// generated: both users have the old default and there is no file.
	oldInstance := func(t *testing.T) (*codersdk.Client, *devConfig) {
		ctx := testutil.Context(t, testutil.WaitLong)
		client := coderdtest.New(t, nil)
		_, err := client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
			Email:    "admin@coder.com",
			Username: "admin",
			Password: oldDefaultPassword,
		})
		require.NoError(t, err)
		resp, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    "admin@coder.com",
			Password: oldDefaultPassword,
		})
		require.NoError(t, err)
		client.SetSessionToken(resp.SessionToken)
		org, err := client.OrganizationByName(ctx, codersdk.DefaultOrganization)
		require.NoError(t, err)
		_, err = client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "member@coder.com",
			Username:        "member",
			Password:        oldDefaultPassword,
			UserLoginType:   codersdk.LoginTypePassword,
			OrganizationIDs: []uuid.UUID{org.ID},
		})
		require.NoError(t, err)

		cfg := &devConfig{configDir: t.TempDir()}
		require.NoError(t, config.Root(cfg.configDir).Session().Write(resp.SessionToken))
		return client, cfg
	}
	requireOldDefaultReplaced := func(t *testing.T, client *codersdk.Client, cfg *devConfig, devClient *codersdk.Client) {
		ctx := testutil.Context(t, testutil.WaitLong)
		admin, ok, err := readPassword(cfg.passwordPath(adminPasswordFile))
		require.NoError(t, err)
		require.True(t, ok)
		member, ok, err := readPassword(cfg.passwordPath(memberPasswordFile))
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, devLogin{password: admin, seeded: true}, cfg.adminLogin)
		require.Equal(t, devLogin{password: member, seeded: true}, cfg.memberLogin)

		require.NoError(t, signIn(t, client, "admin@coder.com", admin))
		require.NoError(t, signIn(t, client, "member@coder.com", member))
		for _, email := range []string{"admin@coder.com", "member@coder.com"} {
			requireUnauthorized(t, signIn(t, client, email, oldDefaultPassword))
		}

		// The password change revoked the session develop started
		// with, so it must have saved a new one.
		_, err = devClient.User(ctx, codersdk.Me)
		require.NoError(t, err)
		saved, err := config.Root(cfg.configDir).Session().Read()
		require.NoError(t, err)
		require.Equal(t, devClient.SessionToken(), saved)
	}

	t.Run("OldDefaultIsReplacedWithASavedSession", func(t *testing.T) {
		t.Parallel()
		client, cfg := oldInstance(t)

		devClient, err := setupFirstUser(testutil.Context(t, testutil.WaitLong), testutil.Logger(t), cfg, client.URL.String())
		require.NoError(t, err)
		requireOldDefaultReplaced(t, client, cfg, devClient)
	})

	t.Run("OldDefaultIsReplacedWithoutASession", func(t *testing.T) {
		t.Parallel()
		client, cfg := oldInstance(t)
		require.NoError(t, os.Remove(filepath.Join(cfg.configDir, "session")))

		devClient, err := setupFirstUser(testutil.Context(t, testutil.WaitLong), testutil.Logger(t), cfg, client.URL.String())
		require.NoError(t, err)
		requireOldDefaultReplaced(t, client, cfg, devClient)
	})

	t.Run("OverrideLeavesTheAdminPasswordAlone", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, cfg := oldInstance(t)
		// validate refuses this value. It is set here only to show that
		// an override stops develop from changing the admin password.
		cfg.password = oldDefaultPassword

		_, err := setupFirstUser(ctx, testutil.Logger(t), cfg, client.URL.String())
		require.NoError(t, err)
		require.NoFileExists(t, cfg.passwordPath(adminPasswordFile))
		require.NoError(t, signIn(t, client, "admin@coder.com", oldDefaultPassword))
		requireUnauthorized(t, signIn(t, client, "member@coder.com", oldDefaultPassword))
		require.NoError(t, signIn(t, client, "member@coder.com", cfg.memberLogin.password))
	})

	t.Run("UsersCreatedElsewhereKeepTheirPasswords", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := testutil.Logger(t)
		client := coderdtest.New(t, nil)
		const elsewhere = "a-password-set-elsewhere-1"

		// With both files removed, this stands in for an instance whose
		// passwords were set some other way.
		first := &devConfig{configDir: t.TempDir(), password: elsewhere}
		_, err := setupFirstUser(ctx, logger, first, client.URL.String())
		require.NoError(t, err)
		require.NoError(t, os.Remove(first.passwordPath(adminPasswordFile)))
		require.NoError(t, os.Remove(first.passwordPath(memberPasswordFile)))

		// With a saved session develop carries on. The server refuses
		// the change because neither password is the old default, so no
		// file is left behind.
		cfg := &devConfig{configDir: first.configDir}
		_, err = setupFirstUser(ctx, logger, cfg, client.URL.String())
		require.NoError(t, err)
		require.Equal(t, devLogin{seeded: true}, cfg.adminLogin)
		require.Equal(t, devLogin{seeded: true}, cfg.memberLogin)
		require.NoFileExists(t, cfg.passwordPath(adminPasswordFile))
		require.NoFileExists(t, cfg.passwordPath(memberPasswordFile))
		require.NoError(t, signIn(t, client, "admin@coder.com", elsewhere))
		require.NoError(t, signIn(t, client, "member@coder.com", first.memberLogin.password))

		// Without a session develop has no password that works.
		require.NoError(t, os.Remove(filepath.Join(cfg.configDir, "session")))
		_, err = setupFirstUser(ctx, logger, &devConfig{configDir: cfg.configDir}, client.URL.String())
		require.ErrorIs(t, err, errUnknownAdminPassword)
		require.NoFileExists(t, cfg.passwordPath(adminPasswordFile))

		// One run with the override is enough. It is stored, so the
		// run after it signs in without the flag.
		_, err = setupFirstUser(ctx, logger, &devConfig{configDir: cfg.configDir, password: elsewhere}, client.URL.String())
		require.NoError(t, err)
		require.NoError(t, os.Remove(filepath.Join(cfg.configDir, "session")))
		_, err = setupFirstUser(ctx, logger, &devConfig{configDir: cfg.configDir}, client.URL.String())
		require.NoError(t, err)
		require.NoError(t, os.Remove(cfg.passwordPath(adminPasswordFile)))
		require.NoError(t, os.Remove(filepath.Join(cfg.configDir, "session")))

		// A stored password that the server refuses gives the same error.
		require.NoError(t, os.WriteFile(cfg.passwordPath(adminPasswordFile), []byte("not-the-right-password-1\n"), 0o600))
		_, err = setupFirstUser(ctx, logger, &devConfig{configDir: cfg.configDir}, client.URL.String())
		require.ErrorIs(t, err, errUnknownAdminPassword)
	})
}
