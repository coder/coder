// Package sessionstore provides CLI session token storage mechanisms.
// Operating system keyring storage is intended to have compatibility with other Coder
// applications (e.g. Coder Desktop, Coder provider for JetBrains Toolbox, etc) so that
// applications can read/write the same credential stored in the keyring.
//
// Note that we aren't using an existing Go package zalando/go-keyring here for a few
// reasons. 1) It prescribes the format of the target credential name in the OS keyrings,
// which makes our life difficult for compatibility with other Coder applications. 2)
// It uses init functions that make it difficult to test with. As a result, the OS
// keyring implementations may be adapted from zalando/go-keyring source (i.e. Windows).
package sessionstore

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/config"
)

// Backend is a storage backend for session tokens.
type Backend interface {
	// Read returns the session token for the given server URL or an error, if any. It
	// will return os.ErrNotExist if no token exists for the given URL.
	Read(serverURL *url.URL) (string, error)
	// Write stores the session token for the given server URL.
	Write(serverURL *url.URL, token string) error
	// Delete removes the session token for the given server URL or an error, if any.
	// It will return os.ErrNotExist error if no token exists to delete.
	Delete(serverURL *url.URL) error
}

var (

	// ErrSetDataTooBig is returned if `keyringProvider.Set` was called with too much data.
	// On macOS: The combination of service, username & password should not exceed ~3000 bytes
	// On Windows: The service is limited to 32KiB while the password is limited to 2560 bytes
	ErrSetDataTooBig = xerrors.New("data passed to Set was too big")

	// ErrNotImplemented represents when keyring usage is not implemented on the current
	// operating system.
	ErrNotImplemented = xerrors.New("not implemented")
)

const (
	// DefaultServiceName is the service name used in keyrings for storing Coder CLI session
	// tokens.
	DefaultServiceName = "coder-v2-credentials"
)

// keyringProvider represents an operating system keyring. The expectation
// is these methods operate on the user/login keyring.
type keyringProvider interface {
	// Set stores the given credential for a service name in the operating system
	// keyring.
	Set(service, credential string) error
	// Get retrieves the credential from the keyring. It must return os.ErrNotExist
	// if the credential is not found.
	Get(service string) ([]byte, error)
	// Delete deletes the credential from the keyring. It must return os.ErrNotExist
	// if the credential is not found.
	Delete(service string) error
}

// credential represents a single credential entry.
type credential struct {
	CoderURL string `json:"coder_url"`
	APIToken string `json:"api_token"`
}

// credentialsMap represents the JSON structure stored in the operating system keyring.
// It supports storing multiple credentials for different server URLs.
type credentialsMap map[string]credential

// normalizeHost returns a normalized version of the URL host for use as a map key.
func normalizeHost(u *url.URL) (string, error) {
	if u == nil || u.Host == "" {
		return "", xerrors.New("nil server URL")
	}
	return strings.TrimSpace(strings.ToLower(u.Host)), nil
}

// parseCredentialsJSON parses the JSON from the keyring into a credentialsMap.
func parseCredentialsJSON(jsonData []byte) (credentialsMap, error) {
	if len(jsonData) == 0 {
		return make(credentialsMap), nil
	}

	var creds credentialsMap
	if err := json.Unmarshal(jsonData, &creds); err != nil {
		return nil, xerrors.Errorf("unmarshal credentials: %w", err)
	}

	return creds, nil
}

// Keyring is a Backend that exclusively stores the session token in the operating
// system keyring. Happy path usage of this type should start with NewKeyring.
// It stores a JSON object in the keyring that supports multiple credentials for
// different server URLs, providing compatibility with Coder Desktop and other Coder
// applications.
type Keyring struct {
	provider    keyringProvider
	serviceName string
}

// NewKeyringWithService creates a Keyring Backend that stores credentials under the
// specified service name. Generally, DefaultServiceName should be provided as the service
// name except in tests which may need parameterization to avoid conflicting keyring use.
func NewKeyringWithService(serviceName string) Keyring {
	return Keyring{
		provider:    operatingSystemKeyring{},
		serviceName: serviceName,
	}
}

func (o Keyring) Read(serverURL *url.URL) (string, error) {
	host, err := normalizeHost(serverURL)
	if err != nil {
		return "", err
	}

	credJSON, err := o.provider.Get(o.serviceName)
	if err != nil {
		return "", err
	}
	if len(credJSON) == 0 {
		return "", os.ErrNotExist
	}

	creds, err := parseCredentialsJSON(credJSON)
	if err != nil {
		return "", xerrors.Errorf("read: parse existing credentials: %w", err)
	}

	// Return the credential for the specified URL
	cred, ok := creds[host]
	if !ok {
		return "", os.ErrNotExist
	}
	return cred.APIToken, nil
}

func (o Keyring) Write(serverURL *url.URL, token string) error {
	host, err := normalizeHost(serverURL)
	if err != nil {
		return err
	}

	existingJSON, err := o.provider.Get(o.serviceName)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return xerrors.Errorf("read existing credentials: %w", err)
	}

	creds, err := parseCredentialsJSON(existingJSON)
	if err != nil {
		return xerrors.Errorf("write: parse existing credentials: %w", err)
	}

	// Upsert the credential for this URL.
	creds[host] = credential{
		CoderURL: host,
		APIToken: token,
	}

	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return xerrors.Errorf("marshal credentials: %w", err)
	}

	err = o.provider.Set(o.serviceName, string(credsJSON))
	if err != nil {
		return xerrors.Errorf("write credentials to keyring: %w", err)
	}
	return nil
}

func (o Keyring) Delete(serverURL *url.URL) error {
	host, err := normalizeHost(serverURL)
	if err != nil {
		return err
	}

	existingJSON, err := o.provider.Get(o.serviceName)
	if err != nil {
		return err
	}

	creds, err := parseCredentialsJSON(existingJSON)
	if err != nil {
		return xerrors.Errorf("failed to parse existing credentials: %w", err)
	}

	if _, ok := creds[host]; !ok {
		return os.ErrNotExist
	}

	delete(creds, host)

	// Delete the entire keyring entry when no credentials remain.
	if len(creds) == 0 {
		return o.provider.Delete(o.serviceName)
	}

	// Write back the updated credentials map.
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return xerrors.Errorf("failed to marshal credentials: %w", err)
	}

	return o.provider.Set(o.serviceName, string(credsJSON))
}

// File is a Backend that exclusively stores the session token in a file on disk.
type File struct {
	config func() config.Root
}

func NewFile(f func() config.Root) *File {
	return &File{config: f}
}

func (f *File) Read(_ *url.URL) (string, error) {
	return f.config().Session().Read()
}

func (f *File) Write(_ *url.URL, token string) error {
	return f.config().Session().Write(token)
}

func (f *File) Delete(_ *url.URL) error {
	return f.config().Session().Delete()
}
