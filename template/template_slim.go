//go:build !embed
// +build !embed

package template

import "github.com/coder/coder/codersdk"

// List returns all embedded templates.
func List() []codersdk.Template {
	return []codersdk.Template{}
}

// Archive returns a tar by template ID.
func Archive(_ string) ([]byte, bool) {
	return nil, false
}
