//go:build !linux

package agentnative

import (
	"context"
	"fmt"
)

func claudeMakePrivate(string) error {
	return fmt.Errorf("Claude requires Linux native state verification: %w", ErrUnsupported)
}
func claudeWritePrivate(string, []byte) error {
	return fmt.Errorf("Claude requires Linux native state verification: %w", ErrUnsupported)
}
func (*Claude) verifyResume(context.Context, Session, string) error {
	return fmt.Errorf("Claude resume requires Linux native process verification: %w", ErrUnsupported)
}

// Shutdown refuses to signal native processes without Linux ownership checks.
func (*Claude) Shutdown(context.Context) error {
	return fmt.Errorf("Claude shutdown requires Linux native process verification: %w", ErrUnsupported)
}
