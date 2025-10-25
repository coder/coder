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
	"net/url"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/config"
)

// Backend is a storage backend for session tokens.
type Backend interface {
	// Read returns the session token and where it is stored for the given server URL,
	// or an error, if any.
	Read() (string, error)
	// Write stores the session token for the given server URL. It returns where the
	// token was stored or an error, if any.
	Write(serverURL *url.URL, token string) error
	// Delete removes any stored session token or an error, if any. It will return
	// os.ErrNotExist error if no token exists to delete.
	Delete() error
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

// keyringProvider represents an operating system keyring. The expectation
// is these methods operate on the user/login keyring.
type keyringProvider interface {
	// Set stores the given credential for a service name in the operating system
	// keyring.
	Set(service, credential string) error
	// Get retrieves the credential from the keyring. It must return os.ErrNotExist
	// if the credential is not found.
	Get(service string) (string, error)
	// Delete deletes the credential from the keyring. It must return os.ErrNotExist
	// if the credential is not found.
	Delete(service string) error
}

// credential represents the JSON structure stored as the value in the operating
// system keyring.
type credential struct {
	CoderURL string `json:"coder_url"`
	APIToken string `json:"api_token"`
}

// generateCredential generates the credential value (a string of JSON) to be stored
// in the operating system keyring.
func generateCredential(u *url.URL, token string) (string, error) {
	if u == nil {
		return "", xerrors.New("nil URL for credential value")
	}
	if u.Host == "" {
		return "", xerrors.New("empty host for credential value")
	}

	cred := credential{
		CoderURL: strings.TrimSpace(strings.ToLower(u.Host)),
		APIToken: token,
	}

	credJSON, err := json.Marshal(cred)
	if err != nil {
		return "", err
	}
	return string(credJSON), nil
}

// Keyring is a Backend that exclusively stores the session token in the operating
// system keyring. Happy path usage of this type should start with NewKeyring.
// It stores a JSON object in the keyring that contains the token for a particular
// server URL for compatibility with Coder Desktop.
type Keyring struct {
	provider keyringProvider
}

func NewKeyring() Keyring {
	return Keyring{provider: operatingSystemKeyring{}}
}

func (o Keyring) Read() (string, error) {
	credJSON, err := o.provider.Get(serviceName)
	if err != nil {
		return "", err
	}
	if credJSON == "" {
		return "", nil
	}

	// Parse the JSON credential to extract just the token
	var cred credential
	if err := json.Unmarshal([]byte(credJSON), &cred); err != nil {
		return "", xerrors.Errorf("failed to parse credential JSON: %w", err)
	}

	return cred.APIToken, nil
}

func (o Keyring) Write(serverURL *url.URL, token string) error {
	cred, err := generateCredential(serverURL, token)
	if err != nil {
		return err
	}
	err = o.provider.Set(serviceName, cred)
	if err != nil {
		return err
	}
	return nil
}

func (o Keyring) Delete() error {
	return o.provider.Delete(serviceName)
}

// File is a Backend that exclusively stores the session token in a file on disk.
type File struct {
	config func() config.Root
}

func NewFile(f func() config.Root) *File {
	return &File{config: f}
}

func (f *File) Read() (string, error) {
	return f.config().Session().Read()
}

func (f *File) Write(_ *url.URL, token string) error {
	return f.config().Session().Write(token)
}

func (f *File) Delete() error {
	return f.config().Session().Delete()
}
