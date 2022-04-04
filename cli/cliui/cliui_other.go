//go:build !darwin
// +build !darwin

package cliui

import "golang.org/x/xerrors"

func removeLineLengthLimit(_ int) (func(), error) {
	return nil, xerrors.New("not implemented")
}
