package chatd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

// ErrInvalidExactSettings rejects a selection that cannot be honored exactly.
var ErrInvalidExactSettings = xerrors.New("invalid exact settings")

// SubmissionOptions opts a mutation into durable retry protection.
type SubmissionOptions struct {
	// ActorContext preserves the initiating actor for on-behalf creation.
	// It is request-local and is never persisted.
	ActorContext  context.Context
	RequestID     uuid.UUID
	ActorID       uuid.UUID
	InputDigest   []byte
	ExactSettings bool
	Receipt       *codersdk.ChatSubmissionReceipt
}

// SubmissionError reports an identity conflict or an uncertain admission.
// Reserved and uncertain identities are never automatically rerun.
type SubmissionError struct {
	Receipt  codersdk.ChatSubmissionReceipt
	Conflict bool
}

func (e *SubmissionError) Error() string {
	if e.Conflict {
		return "submission identity was already used with different input"
	}
	if e.Receipt.State == "rejected" {
		return fmt.Sprintf("submission %s rejected: %s", e.Receipt.ID, e.Receipt.Error)
	}
	if e.Receipt.State == "uncertain" {
		return fmt.Sprintf("submission %s outcome is uncertain and needs operator reconciliation: %s", e.Receipt.ID, e.Receipt.Error)
	}
	return fmt.Sprintf("submission %s (request %s) outcome is %s; do not repeat with another request identity", e.Receipt.ID, e.Receipt.RequestID, e.Receipt.State)
}

func submissionReceipt(row database.ChatSubmission) (*codersdk.ChatSubmissionReceipt, error) {
	receipt := &codersdk.ChatSubmissionReceipt{
		ID: row.ID, RequestID: row.RequestID, ChatID: row.ChatID, State: row.State,
		Error: row.Error, MessageID: row.MessageID.Int64, QueuedMessageID: row.QueuedMessageID.Int64,
	}
	if err := json.Unmarshal(row.Settings, &receipt.Settings); err != nil {
		return nil, xerrors.Errorf("decode submission settings: %w", err)
	}
	return receipt, nil
}

func (p *Server) prepareSubmission(ctx context.Context, opts *SubmissionOptions, chat database.Chat, kind string, modelID uuid.UUID, effort *string) (bool, error) {
	if opts == nil {
		return false, nil
	}
	if opts.RequestID == uuid.Nil || opts.ActorID == uuid.Nil || len(opts.InputDigest) != 32 {
		return false, xerrors.New("submission requires request ID, actor and SHA-256 input digest")
	}
	journalCtx := ctx
	if opts.ActorContext != nil {
		journalCtx = opts.ActorContext
	}
	replayed, err := p.ReplaySubmission(ctx, opts, chat.OrganizationID, kind)
	if replayed || err != nil {
		return replayed, err
	}
	var settings *codersdk.ChatExactSettings
	if opts.ExactSettings {
		if chat.Mode.Valid && chat.Mode.ChatMode == database.ChatModeComputerUse {
			return false, xerrors.Errorf("%w: computer-use mode substitutes its own model", ErrInvalidExactSettings)
		}
		settings, err = resolveExactSettings(ctx, p.db, chat, modelID, effort)
		if err != nil {
			return false, err
		}
	}
	rawSettings, err := json.Marshal(settings)
	if err != nil {
		return false, err
	}
	var row database.ChatSubmission
	err = p.db.InTx(func(tx database.Store) error {
		if kind == "message" {
			current, err := tx.GetChatByIDForUpdate(ctx, chat.ID)
			if err != nil {
				return err
			}
			if err := p.checkExistingChatWorkspaceAdmission(ctx, tx, current); err != nil {
				return err
			}
		} else if chat.WorkspaceID.Valid {
			if err := p.CheckWorkspaceAdmission(ctx, tx, chat.WorkspaceID.UUID); err != nil {
				return err
			}
		}
		var err error
		row, err = tx.InsertChatSubmission(journalCtx, database.InsertChatSubmissionParams{
			ID: uuid.New(), OrganizationID: chat.OrganizationID, ActorID: opts.ActorID,
			OwnerID: chat.OwnerID, RequestID: opts.RequestID, InputDigest: opts.InputDigest,
			Kind: kind, ChatID: chat.ID, Settings: rawSettings,
		})
		return err
	}, nil)
	if xerrors.Is(err, sql.ErrNoRows) {
		previous, err := p.db.GetChatSubmission(journalCtx, database.GetChatSubmissionParams{OrganizationID: chat.OrganizationID, ActorID: opts.ActorID, RequestID: opts.RequestID})
		if err != nil {
			return false, err
		}
		return replaySubmission(opts, previous, kind)
	}
	if err != nil {
		return false, err
	}
	opts.Receipt, err = submissionReceipt(row)
	return false, err
}

// ReplaySubmission recovers an existing actor-owned identity before mutable
// admission policy. The caller must still authorize the requested chat operation.
func (p *Server) ReplaySubmission(ctx context.Context, opts *SubmissionOptions, organizationID uuid.UUID, kind string) (bool, error) {
	if opts == nil {
		return false, nil
	}
	if opts.RequestID == uuid.Nil || opts.ActorID == uuid.Nil || len(opts.InputDigest) != 32 {
		return false, xerrors.New("submission requires request ID, actor and SHA-256 input digest")
	}
	if opts.ActorContext != nil {
		ctx = opts.ActorContext
	}
	row, err := p.db.GetChatSubmission(ctx, database.GetChatSubmissionParams{OrganizationID: organizationID, ActorID: opts.ActorID, RequestID: opts.RequestID})
	if xerrors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return replaySubmission(opts, row, kind)
}

func replaySubmission(opts *SubmissionOptions, row database.ChatSubmission, kind string) (bool, error) {
	receipt, err := submissionReceipt(row)
	if err != nil {
		return false, err
	}
	opts.Receipt = receipt
	if !bytes.Equal(row.InputDigest, opts.InputDigest) || row.Kind != kind {
		return false, &SubmissionError{Receipt: *receipt, Conflict: true}
	}
	if row.State != "accepted" {
		return false, &SubmissionError{Receipt: *receipt}
	}
	return true, nil
}

func completeSubmission(ctx context.Context, store database.Store, opts *SubmissionOptions, organizationID uuid.UUID, messageID, queuedID int64) error {
	if opts == nil {
		return nil
	}
	journalCtx := ctx
	if opts.ActorContext != nil {
		journalCtx = opts.ActorContext
	}
	row, err := store.CompleteChatSubmission(journalCtx, database.CompleteChatSubmissionParams{
		OrganizationID: organizationID, ActorID: opts.ActorID, RequestID: opts.RequestID,
		MessageID:       sql.NullInt64{Int64: messageID, Valid: messageID != 0},
		QueuedMessageID: sql.NullInt64{Int64: queuedID, Valid: queuedID != 0},
	})
	if err != nil {
		return err
	}
	opts.Receipt, err = submissionReceipt(row)
	return err
}

func resolveExactSettings(ctx context.Context, store database.Store, chat database.Chat, modelID uuid.UUID, effort *string) (*codersdk.ChatExactSettings, error) {
	config, err := store.GetEnabledChatModelConfigByID(ctx, modelID)
	if err != nil {
		return nil, err
	}
	if config.OrganizationID != chat.OrganizationID {
		return nil, ErrInvalidModelConfigID
	}
	// The caller can use this organization-scoped model. Provider metadata is
	// needed only to validate its exact settings; credentials are never returned.
	//nolint:gocritic // Reading the model's provider is an internal chatd operation.
	provider, err := store.GetAIProviderByID(dbauthz.AsChatd(ctx), config.AIProviderID.UUID)
	if err != nil {
		return nil, err
	}
	if !provider.Enabled {
		return nil, xerrors.New("selected provider is disabled")
	}
	providerName, modelName, err := chatprovider.ResolveModelWithProviderHint(config.Model, string(provider.Type))
	if err != nil {
		return nil, err
	}
	callConfig, err := parseModelConfigOptions(config.Options)
	if err != nil {
		return nil, err
	}
	resolved, err := chatprovider.ResolveExactReasoningEffort(providerName, modelName, callConfig, effort)
	if err != nil {
		return nil, xerrors.Errorf("%w: %s", ErrInvalidExactSettings, err)
	}
	return &codersdk.ChatExactSettings{
		ModelConfigID: config.ID, Provider: providerName, Model: modelName,
		ReasoningEffort: resolved,
	}, nil
}

// validateExactTurn checks the actual turn's immutable selection again after
// queue promotion and config reload. Configuration changes cannot downgrade it.
func (p *Server) validateExactTurn(ctx context.Context, chatID uuid.UUID, resolved resolvedModelCall) error {
	raw, err := p.db.GetLatestChatSubmissionSettings(ctx, chatID)
	if xerrors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var selected *codersdk.ChatExactSettings
	if err := json.Unmarshal(raw, &selected); err != nil {
		return xerrors.Errorf("decode exact turn settings: %w", err)
	}
	if selected == nil {
		return nil
	}
	if selected.ModelConfigID != resolved.dbConfig.ID || selected.Provider != resolved.resolvedProvider || selected.Model != resolved.resolvedModel {
		return xerrors.New("exact model selection changed since submission")
	}
	effective, err := chatprovider.ResolveExactReasoningEffort(resolved.resolvedProvider, resolved.resolvedModel, resolved.callConfig, selected.ReasoningEffort)
	if err != nil {
		return err
	}
	if (effective == nil) != (selected.ReasoningEffort == nil) || (effective != nil && *effective != *selected.ReasoningEffort) {
		return xerrors.New("exact reasoning settings changed since submission")
	}
	return nil
}

// checkExistingChatWorkspaceAdmission permits recovery from a deleted binding.
// The caller must authorize and lock the existing chat in this transaction.
func (p *Server) checkExistingChatWorkspaceAdmission(ctx context.Context, tx database.Store, chat database.Chat) error {
	if p.checkWorkspaceAdmission == nil || !chat.WorkspaceID.Valid {
		return nil
	}
	//nolint:gocritic // This internal state check follows existing chat authorization.
	workspace, err := tx.GetWorkspaceByID(dbauthz.AsSystemRestricted(ctx), chat.WorkspaceID.UUID)
	if err != nil {
		return xerrors.Errorf("fetch chat workspace for admission: %w", err)
	}
	if workspace.Deleted {
		return nil
	}
	return p.CheckWorkspaceAdmission(ctx, tx, chat.WorkspaceID.UUID)
}

// CheckWorkspaceAdmission serializes admission with workspace cleanup. The
// caller must supply the same transaction handle used to commit the mutation.
func (p *Server) CheckWorkspaceAdmission(ctx context.Context, tx database.Store, workspaceID uuid.UUID) error {
	if p.checkWorkspaceAdmission == nil {
		return nil
	}
	return p.checkWorkspaceAdmission(ctx, tx, workspaceID)
}

// finishFailedSubmission records known rejection or an unresolved outcome after
// this handler stops admission. Uncertain outcomes remain protected activity.
func (p *Server) finishFailedSubmission(ctx context.Context, opts *SubmissionOptions, organizationID uuid.UUID, cause error, state string) {
	if opts == nil || opts.Receipt == nil || cause == nil {
		return
	}
	journalCtx := ctx
	if opts.ActorContext != nil {
		journalCtx = opts.ActorContext
	}
	journalCtx, cancel := context.WithTimeout(context.WithoutCancel(journalCtx), 5*time.Second)
	defer cancel()
	message := cause.Error()
	if dispatchErr, ok := errors.AsType[*dispatch.Error](cause); ok {
		message = fmt.Sprintf("hook dispatch %s (%s): %s", dispatchErr.DispatchID, dispatchErr.Class, message)
	}
	row, err := p.db.FinishChatSubmission(journalCtx, database.FinishChatSubmissionParams{
		ID: opts.Receipt.ID, State: state,
		OrganizationID: organizationID, ActorID: opts.ActorID, RequestID: opts.RequestID, Error: message,
	})
	if err == nil {
		opts.Receipt, _ = submissionReceipt(row)
	}
}

func knownSubmissionRejection(err error) bool {
	var denied *chathooks.UserPromptDeniedError
	var rolledBack *chatstate.RolledBackError
	return errors.As(err, &denied) || errors.As(err, &rolledBack)
}
