// Package httpapiconstraints contain types that can be used and implemented
// across the application to return specific HTTP status codes without pulling
// in large dependency trees.
package httpapiconstraints

// IsUnauthorizedError is an interface that can be implemented in other packages
// in order to return 404.
type IsUnauthorizedError interface {
	IsUnauthorized() bool
}
