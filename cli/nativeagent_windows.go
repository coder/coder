package cli

import "errors"

func nativeReadPrivate(string) ([]byte, error) {
	return nil, errors.New("native agents require a Unix workspace")
}

func nativeCheckPrivateDirectory(string) error {
	return errors.New("native agents require a Unix workspace")
}

func nativePrivateDirectory(string) error {
	return errors.New("native agents require a Unix workspace")
}
