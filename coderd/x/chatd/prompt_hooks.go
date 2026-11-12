package chatd

import (
	"cmp"
	"context"
	"errors"

	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
)

// promptHooksDeniedFallback is the chat error when a hook denies a message
// and sends no user message. The HTTP sites use the same text.
const promptHooksDeniedFallback = "Chat message denied by lifecycle hook."

// firstPromptHooksPending returns the oldest message that still waits for its
// user_prompt_submit hooks.
func firstPromptHooksPending(messages []database.ChatMessage) (database.ChatMessage, bool) {
	for _, message := range messages {
		if message.PromptHooksPending {
			return message, true
		}
	}
	return database.ChatMessage{}, false
}

// promptHooksOutcome is what the hooks decided about one message.
type promptHooksOutcome struct {
	// content is the rewritten content. It is not valid when the hooks left
	// the message as saved.
	content pqtype.NullRawMessage
	// denial is the text shown as the chat error when a hook denied the
	// message.
	denial string
}

// resolvePromptHooks runs the user_prompt_submit hooks for the oldest message
// that waits for them, before anything else in the step can show that
// message to a model. No transaction runs a hook. One short transaction then
// applies the outcome and clears the flag.
//
// It returns done when the step is over. After the message is settled the
// history change starts a new generation task, as it does after a steer
// delivery, so the task must not continue. A denial deletes the message and
// fails the chat in the same transaction.
func (s *taskStarter) resolvePromptHooks(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	chat database.Chat,
	messages []database.ChatMessage,
) (done bool, err error) {
	message, ok := firstPromptHooksPending(messages)
	if !ok {
		return false, nil
	}
	outcome, err := s.dispatchPromptHooks(ctx, input, chat, message)
	if err == nil {
		if outcome.denial != "" {
			return true, s.denyPromptHooks(ctx, machine, input, message, outcome.denial)
		}
		err = s.applyPromptHooks(ctx, machine, input, message, outcome)
	}
	if err == nil {
		return true, nil
	}
	if errors.Is(err, errTaskExpectedExit) || errors.Is(err, errTaskRetryable) {
		return true, xerrors.Errorf("prompt hooks of message %d: %w", message.ID, err)
	}
	return true, s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
}

// dispatchPromptHooks asks the hook consumer about one message. With hooks
// off it asks nobody and leaves the message as saved.
func (s *taskStarter) dispatchPromptHooks(
	ctx context.Context,
	input chatWorkerTaskStartInput,
	chat database.Chat,
	message database.ChatMessage,
) (promptHooksOutcome, error) {
	if !s.server.hooks.Enabled() {
		return promptHooksOutcome{}, nil
	}
	parts, err := chatprompt.ParseContent(message)
	if err != nil {
		return promptHooksOutcome{}, xerrors.Errorf("parse message %d for its prompt hooks: %w", message.ID, err)
	}
	prompt, err := chathooks.UserPromptMessage(parts)
	if err != nil {
		return promptHooksOutcome{}, err
	}
	// The message is already accepted, so it draws on the capacity that is
	// reserved for admitted work, not on the admission gate that new sends use.
	result, err := s.server.hooks.Trigger(ctx, chathooks.ChatFor(chat, input.hookTurnID()), prompt, agenthooks.EventUserPromptSubmit, dispatch.CapacityClassGeneration)
	if err != nil {
		if denied, ok := errors.AsType[*chathooks.UserPromptDeniedError](chathooks.UserPromptDenial(err)); ok {
			return promptHooksOutcome{denial: cmp.Or(denied.UserMessage, promptHooksDeniedFallback)}, nil
		}
		return promptHooksOutcome{}, chathooks.GenerationDispatchError(agenthooks.EventUserPromptSubmit, err)
	}
	composed, overridden, err := chathooks.ComposeUserPromptContent(parts, result)
	if err != nil {
		return promptHooksOutcome{}, err
	}
	if !overridden && len(chathooks.UserPromptParts(result)) == 0 {
		return promptHooksOutcome{}, nil
	}
	content, err := chatprompt.MarshalParts(composed)
	if err != nil {
		return promptHooksOutcome{}, xerrors.Errorf("marshal message %d after its prompt hooks: %w", message.ID, err)
	}
	return promptHooksOutcome{content: content}, nil
}

// applyPromptHooks commits the outcome of an allowed message.
func (s *taskStarter) applyPromptHooks(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	message database.ChatMessage,
	outcome promptHooksOutcome,
) error {
	var committed database.Chat
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := loadChatForGeneration(ctx, store, input, generationAttemptNotRequired); err != nil {
			return xerrors.Errorf("load chat for prompt hooks: %w", err)
		}
		if _, err := tx.ResolvePromptHooks(chatstate.ResolvePromptHooksInput{
			MessageID: message.ID,
			Content:   outcome.content,
		}); err != nil {
			return err
		}
		var err error
		committed, err = store.GetChatByID(ctx, input.ChatID)
		return err
	})
	if err != nil {
		return normalizeTaskTransitionError(err, "apply prompt hooks")
	}
	s.routeStateHint(ctx, stateUpdateFromChat(committed))
	return nil
}

// denyPromptHooks deletes a denied message and fails the chat with the reason
// in one transaction, so a crash cannot leave the message deleted and the
// chat running. Messages queued behind it stay queued.
func (s *taskStarter) denyPromptHooks(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	message database.ChatMessage,
	reason string,
) error {
	cause := chaterror.WithClassification(
		&chathooks.UserPromptDeniedError{UserMessage: reason},
		chaterror.ClassifiedError{Message: reason, Kind: codersdk.ChatErrorKindHookDenied},
	)
	return s.finishGenerationErrorWith(ctx, machine, input, cause, generationAttemptNotRequired, func(tx *chatstate.Tx) error {
		_, err := tx.ResolvePromptHooks(chatstate.ResolvePromptHooksInput{MessageID: message.ID, Deny: true})
		return err
	})
}
