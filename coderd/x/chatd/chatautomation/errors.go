package chatautomation

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

var (
	// ErrCreateForbidden means that the actor cannot create the automation.
	ErrCreateForbidden = xerrors.New("not allowed to create the chat automation")
	// ErrTargetChatForbidden means that the target chat does not exist, or
	// that the actor cannot read it or does not own it.
	ErrTargetChatForbidden = xerrors.New("the target chat is not a chat that the actor owns")
	// ErrLimitReached means that the owner already has
	// MaxAutomationsPerOwner automations.
	ErrLimitReached = xerrors.New("the owner has the most chat automations allowed")
	// ErrReferenceUnavailable means that a chat, model configuration or
	// project of the automation was deleted while it was created.
	ErrReferenceUnavailable = xerrors.New("a chat, model configuration or project of the chat automation no longer exists")
)

// InvalidRequestError lists the fields of a create request that Create
// rejects. No detail quotes the prompt, so a caller can return the list as it
// is.
type InvalidRequestError struct {
	Validations []codersdk.ValidationError
}

func (e *InvalidRequestError) Error() string {
	return fmt.Sprintf("invalid chat automation: %d fields rejected", len(e.Validations))
}
