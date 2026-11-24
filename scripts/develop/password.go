//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cryptorand"
)

const (
	adminPasswordFile       = "admin-password"
	memberPasswordFile      = "member-password"
	generatedPasswordLength = 32
	// oldDefaultPassword was the password of every develop instance. It
	// is public, so it is refused as an override.
	oldDefaultPassword = "SomeSecurePassword!"
)

func (c *devConfig) passwordPath(name string) string {
	return filepath.Join(c.configDir, name)
}

// readPassword reports ok as false when the file does not exist.
func readPassword(path string) (password string, ok bool, err error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, xerrors.Errorf("read password file %s: %w", path, err)
	}
	// Only the newline that storePassword adds is removed, because a
	// password can start or end with a space.
	password = strings.TrimSuffix(string(raw), "\n")
	if password == "" {
		return "", false, xerrors.Errorf("password file %s is empty: put the password back, or delete the file and run with --db-reset", path)
	}
	return password, true, nil
}

// loadOrCreatePassword never replaces a stored password, because a user
// in the database was created with it.
func loadOrCreatePassword(path string) (string, error) {
	if password, ok, err := readPassword(path); err != nil || ok {
		return password, err
	}
	password, err := cryptorand.String(generatedPasswordLength)
	if err != nil {
		return "", xerrors.Errorf("generate password: %w", err)
	}
	if err := storePassword(path, password); err != nil {
		return "", err
	}
	return password, nil
}

func replacePassword(path, password string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return xerrors.Errorf("remove password file %s: %w", path, err)
	}
	return storePassword(path, password)
}

func storePassword(path, password string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return xerrors.Errorf("create directory for %s: %w", path, err)
	}
	// O_EXCL makes a second develop that starts at the same moment fail
	// instead of replacing a password the first one is about to use.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return xerrors.Errorf("create password file %s: %w", path, err)
	}
	if _, err := f.WriteString(password + "\n"); err != nil {
		_ = f.Close()
		return xerrors.Errorf("write password file %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return xerrors.Errorf("close password file %s: %w", path, err)
	}
	return nil
}
