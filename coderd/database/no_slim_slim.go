//go:build slim

package database

const (
	// This re-declaration will result in a compilation error and is present to
	// prevent increasing the slim binary size by importing this package,
	// directly or indirectly.
	//
	// no_slim_slim.go:7:2: _DO_NOT_IMPORT_THIS_PACKAGE_IN_SLIM_BUILDS redeclared in this block
	// 	no_slim.go:4:2: other declaration of _DO_NOT_IMPORT_THIS_PACKAGE_IN_SLIM_BUILDS
	//nolint:revive,unused
	_DO_NOT_IMPORT_THIS_PACKAGE_IN_SLIM_BUILDS = "DO_NOT_IMPORT_THIS_PACKAGE_IN_SLIM_BUILDS"
)
