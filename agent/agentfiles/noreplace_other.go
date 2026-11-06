//go:build !linux

package agentfiles

// renameNoReplace publishes tmp at path unless path exists. See
// linkNoReplace.
func renameNoReplace(tmp, path string) error {
	return linkNoReplace(tmp, path)
}
