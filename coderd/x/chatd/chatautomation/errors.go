package chatautomation

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// ErrCreateForbidden means that the actor cannot create an automation.
var ErrCreateForbidden = xerrors.New("not allowed to create a chat automation")

// ErrOwnerUnavailable means that the owner is deleted or is a system user, so
// no automation can belong to it.
var ErrOwnerUnavailable = xerrors.New("the owner cannot own chat automations")

// InvalidRequestError lists every field of a create request that Create
// rejects. Each detail is static text or a limit and never quotes the prompt
// or the webhook secret, so a caller can return the list as it is.
type InvalidRequestError struct {
	Validations []codersdk.ValidationError
}

func (e *InvalidRequestError) Error() string {
	return fmt.Sprintf("invalid chat automation: %d fields rejected", len(e.Validations))
}

// LimitError means that the owner already has the most automations that the
// deployment allows.
type LimitError struct {
	Max int64
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("an owner can have at most %d chat automations", e.Max)
}
