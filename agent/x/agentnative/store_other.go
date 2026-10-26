//go:build !linux

package agentnative

// Native session management is Linux-only. Keep the agent package buildable on
// other platforms without weakening private-file guarantees there.
type privateStamp struct{}
type privateDirectory struct{}

func openPrivateDirectory(string, privateDirectoryMode) (*privateDirectory, error) {
	return nil, ErrUnsupported
}
func (*privateDirectory) acquire() error { return ErrUnsupported }
func (*privateDirectory) read(string, int) ([]byte, *privateStamp, error) {
	return nil, nil, ErrUnsupported
}
func (*privateDirectory) write(string, []byte, *privateStamp) (*privateStamp, error) {
	return nil, ErrUnsupported
}
func (*privateDirectory) close() error { return nil }
