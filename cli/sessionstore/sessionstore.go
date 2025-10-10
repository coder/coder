// Package sessionstore provides CLI session token storage mechanisms.
package sessionstore

import (
	"errors"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/zalando/go-keyring"

	"github.com/coder/coder/v2/cli/config"
)

// Backend is a storage backend for session tokens. It's similar to keyring.Keyring
// but intended to allow more flexible types and avoid global behaviors in the keyring
// package (e.g. using the keyring package mock modifies a package global).
type Backend interface {
	// Read returns the session token and where it is stored for the given server URL,
	// or an error, if any.
	Read(conf config.Root, serverURL *url.URL) (string, Location, error)
	// Write stores the session token for the given server URL. It returns where the
	// token was stored or an error, if any.
	Write(conf config.Root, serverURL *url.URL, token string) (Location, error)
	// Delete removes any stored session token or an error, if any. It will return
	// os.ErrNotExist error if no token exists to delete.
	Delete(conf config.Root, serverURL *url.URL) error
	// PreferredLocation returns the preferred token storage Location of this Backend.
	PreferredLocation() Location
}

type keyringProvider keyring.Keyring

type operatingSystemKeyring struct{}

func (operatingSystemKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (operatingSystemKeyring) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (operatingSystemKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

const (
	servicePrefix  = "coder-cli:"
	keyringAccount = "session"
)

// Location represents where a session token is stored.
type Location string

const (
	LocationNone    Location = "none"
	LocationFile    Location = "file"
	LocationKeyring Location = "keyring"
)

func serviceName(u *url.URL) string {
	if u == nil || u.Host == "" {
		return servicePrefix + "default"
	}
	host := strings.TrimSpace(strings.ToLower(u.Host))
	return servicePrefix + host
}

// KeyringWithFallback is a Backend that prefers keyring storage and falls back to file
// storage if the operating system keyring is unavailable. Happy path usage of this
// type should start with NewKeyringWithFileFallback.
type KeyringWithFallback struct {
	keyringProvider keyringProvider
}

func NewKeyringWithFileFallback() KeyringWithFallback {
	return NewKeyringProviderWithFileFallback(operatingSystemKeyring{})
}

func NewKeyringProviderWithFileFallback(provider keyringProvider) KeyringWithFallback {
	return KeyringWithFallback{keyringProvider: provider}
}

// Read prefers reading the token from the operating system keyring and falls back
// to file storage if the operating system keyring is unavailable.
func (o KeyringWithFallback) Read(conf config.Root, serverURL *url.URL) (string, Location, error) {
	svc := serviceName(serverURL)
	tok, err := o.keyringProvider.Get(svc, keyringAccount)
	if err == nil && tok != "" {
		return tok, LocationKeyring, nil
	}
	// Fallback to file storage.
	return File{}.Read(conf, serverURL)
}

// Write prefers storing the token in the operating system keyring and falls back
// to file storage if the keyring operation fails.
func (o KeyringWithFallback) Write(conf config.Root, serverURL *url.URL, token string) (Location, error) {
	svc := serviceName(serverURL)
	err := o.keyringProvider.Set(svc, keyringAccount, token)
	if err == nil {
		// Best effort: remove plaintext file if it exists.
		_ = conf.Session().Delete()
		return LocationKeyring, nil
	}
	// Fallback to file storage.
	return File{}.Write(conf, serverURL, token)
}

// Delete removes any stored session token from both the keyring and file.
func (o KeyringWithFallback) Delete(conf config.Root, serverURL *url.URL) error {
	svc := serviceName(serverURL)

	keyringErr := o.keyringProvider.Delete(svc, keyringAccount)
	if keyringErr != nil {
		if errors.Is(keyringErr, keyring.ErrNotFound) {
			// Make the error handling for keyring/file backends uniform for the caller.
			keyringErr = os.ErrNotExist
		} else if runtime.GOOS == "linux" {
			// It's possible the Linux keyring dependencies are not installed (e.g. go-keyring
			// on Linux requires D-Bus and GNOME Keyring). In this case, when keyring.Delete
			// fails we can't reliably differentiate between the dependencies not being
			// installed (customer doesn't want to opt into using the OS keyring) or a genuine
			// problem deleting the token. As a result, we unfortunately silently swallow the
			// error here to prevent bubbling up a misleading error.
			//
			// This could be addressed by alternative approaches such as requiring use of the
			// OS keyring unless a flag/env var specifies exclusive storage in a file on disk.
			// This approach would require Linux users to have the dependencies installed when
			// updating CLI versions.
			keyringErr = nil
		}
	}

	fileErr := File{}.Delete(conf, serverURL)
	switch {
	case fileErr != nil && keyringErr == nil:
		// Happy path when using the keyring. We could drill down into fileErr, but for
		// simplicity we will assume the error is because use of the keyring means the
		// file didn't exist. See the Linux exception to this above.
		return nil
	case fileErr == nil && keyringErr != nil:
		// Deleted the file because of a problem with the OS keyring. The file may have
		// been created as a result of falling back to disk, or it may have already
		// existed.
		return nil
	default:
		return errors.Join(keyringErr, fileErr)
	}
}

func (KeyringWithFallback) PreferredLocation() Location { return LocationKeyring }

// File is a Backend that exclusively stores the session token in a file on disk.
type File struct{}

func (File) Read(conf config.Root, _ *url.URL) (string, Location, error) {
	tok, err := conf.Session().Read()
	if err != nil {
		return "", LocationNone, err
	}
	return tok, LocationFile, nil
}

func (File) Write(conf config.Root, _ *url.URL, token string) (Location, error) {
	if err := conf.Session().Write(token); err != nil {
		return LocationNone, err
	}
	return LocationFile, nil
}

func (File) Delete(conf config.Root, _ *url.URL) error {
	return conf.Session().Delete()
}

func (File) PreferredLocation() Location { return LocationFile }
