//go:build windows

package sessionstore

import (
	"errors"
	"os"
	"syscall"

	"github.com/danieljoos/wincred"
)

// operatingSystemKeyring implements keyringProvider and uses Windows Credential Manager.
// It is largely adapted from the zalando/go-keyring package.
type operatingSystemKeyring struct{}

func (operatingSystemKeyring) Set(service, credential string) error {
	// password may not exceed 2560 bytes (https://github.com/jaraco/keyring/issues/540#issuecomment-968329967)
	if len(credential) > 2560 {
		return ErrSetDataTooBig
	}

	// service may not exceed 512 bytes (might need more testing)
	if len(service) >= 512 {
		return ErrSetDataTooBig
	}

	// service may not exceed 32k but problems occur before that
	// so we limit it to 30k
	if len(service) > 1024*30 {
		return ErrSetDataTooBig
	}

	cred := wincred.NewGenericCredential(service)
	cred.CredentialBlob = []byte(credential)
	cred.Persist = wincred.PersistLocalMachine
	return cred.Write()
}

func (operatingSystemKeyring) Get(service string) ([]byte, error) {
	cred, err := wincred.GetGenericCredential(service)
	if err != nil {
		if errors.Is(err, syscall.ERROR_NOT_FOUND) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	return cred.CredentialBlob, nil
}

func (operatingSystemKeyring) Delete(service string) error {
	cred, err := wincred.GetGenericCredential(service)
	if err != nil {
		if errors.Is(err, syscall.ERROR_NOT_FOUND) {
			return os.ErrNotExist
		}
		return err
	}
	return cred.Delete()
}
