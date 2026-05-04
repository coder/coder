//go:build slim

package chatexec

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// ErrRequiresAction is returned when the runner parks for external action.
var ErrRequiresAction = xerrors.New("chat requires action")

// Executor is a slim-build placeholder. Agent chat execution is disabled in
// slim builds because it depends on the full chat runtime.
type Executor struct{}

// New returns a placeholder executor for slim builds.
func New(
	_ any,
	_ slog.Logger,
	_ any,
) *Executor {
	return &Executor{}
}

// Execute reports that agent chat execution is unavailable in slim builds.
func (*Executor) Execute(context.Context, uuid.UUID) error {
	return xerrors.New("agent chat runner is unavailable in slim builds")
}
