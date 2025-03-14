//go:build !linux
// +build !linux
package agentexec
import 
import "errors"
func CLI() error {
	return errors.New("agent-exec is only supported on Linux")
}
