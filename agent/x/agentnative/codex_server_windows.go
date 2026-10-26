//go:build windows

package agentnative

import "context"

func (*Codex) privateDirectory(bool) error { return ErrUnsupported }
func (*Codex) boundSocket(context.Context) (codexSocketPeer, error) {
	return codexSocketPeer{}, ErrUnsupported
}
func (*Codex) checkServerStart(context.Context) error { return ErrUnsupported }
func (*Codex) startServer() error                     { return ErrUnsupported }
func (*Codex) finalizeServerIdentity() error          { return ErrUnsupported }
func (*Codex) killOwnedGroup() error                  { return ErrUnsupported }
func (*Codex) stopOwned(context.Context) error        { return ErrUnsupported }
