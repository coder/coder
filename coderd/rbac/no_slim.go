//go:build slim

package rbac

const (
	// This line fails to compile, preventing this package from being imported
	// in slim builds.
	_DO_NOT_IMPORT_THIS_PACKAGE_IN_SLIM_BUILDS = _DO_NOT_IMPORT_THIS_PACKAGE_IN_SLIM_BUILDS
)
