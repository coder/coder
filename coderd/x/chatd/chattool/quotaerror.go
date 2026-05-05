package chattool

import (
	"context"
	"errors"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

const workspaceQuotaErrorTitle = "Workspace quota reached"

type buildFailureAction string

const (
	buildFailureActionCreate buildFailureAction = "create"
	buildFailureActionStart  buildFailureAction = "start"
)

type workspaceBuildError struct {
	message string
	code    codersdk.JobErrorCode
}

func (e *workspaceBuildError) Error() string {
	return e.message
}

func buildErrorCode(err error) codersdk.JobErrorCode {
	var buildErr *workspaceBuildError
	if errors.As(err, &buildErr) {
		return buildErr.code
	}
	return ""
}

// quotaErrorResult is the structured response returned when a workspace
// build fails because the user's workspace quota is exhausted.
type quotaErrorResult struct {
	ErrorCode codersdk.JobErrorCode `json:"error_code"`
	// Error is the raw build failure string used for debugging and
	// frontend error detection.
	Error string `json:"error"`
	// Title is a short user-facing summary.
	Title string `json:"title"`
	// Message is a user-facing explanation of why the action failed.
	Message string `json:"message"`
	// NextSteps gives the model recovery guidance to relay to the user.
	NextSteps []string           `json:"next_steps"`
	BuildID   string             `json:"build_id,omitempty"`
	Quota     *quotaErrorDetails `json:"quota,omitempty"`
}

type quotaErrorDetails struct {
	CreditsConsumed int64 `json:"credits_consumed"`
	Budget          int64 `json:"budget"`
}

func newQuotaError(
	msg string,
	buildID uuid.UUID,
	action buildFailureAction,
	quota *quotaErrorDetails,
) quotaErrorResult {
	message := "Coder could not create this workspace because your workspace quota is full."
	if action == buildFailureActionStart {
		message = "Coder could not start this workspace because your workspace quota is full."
	}

	r := quotaErrorResult{
		ErrorCode: codersdk.InsufficientQuota,
		Error:     msg,
		Title:     workspaceQuotaErrorTitle,
		Message:   message,
		NextSteps: []string{
			"Delete a workspace you no longer need to free quota.",
			"Ask an administrator to raise your group quota allowance.",
		},
		Quota: quota,
	}
	if buildID != uuid.Nil {
		r.BuildID = buildID.String()
	}
	return r
}

func workspaceQuotaDetails(
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
) *quotaErrorDetails {
	if db == nil || ownerID == uuid.Nil || organizationID == uuid.Nil {
		return nil
	}

	consumed, err := db.GetQuotaConsumedForUser(ctx, database.GetQuotaConsumedForUserParams{
		OwnerID:        ownerID,
		OrganizationID: organizationID,
	})
	if err != nil {
		logger.Debug(ctx, "failed to load consumed workspace quota",
			slog.F("owner_id", ownerID),
			slog.F("organization_id", organizationID),
			slog.Error(err),
		)
		return nil
	}
	budget, err := db.GetQuotaAllowanceForUser(ctx, database.GetQuotaAllowanceForUserParams{
		UserID:         ownerID,
		OrganizationID: organizationID,
	})
	if err != nil {
		logger.Debug(ctx, "failed to load workspace quota allowance",
			slog.F("owner_id", ownerID),
			slog.F("organization_id", organizationID),
			slog.Error(err),
		)
		return nil
	}
	return &quotaErrorDetails{
		CreditsConsumed: consumed,
		Budget:          budget,
	}
}

func quotaErrorToolResponse(
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
	msg string,
	buildID uuid.UUID,
	action buildFailureAction,
) fantasy.ToolResponse {
	quota := workspaceQuotaDetails(ctx, logger, db, ownerID, organizationID)
	return marshalToolResponse(newQuotaError(msg, buildID, action, quota))
}

// buildFailureToolResponse keeps build failures as JSON carried in a normal
// text tool response. The chatprompt pipeline flattens IsError responses into
// a single string and drops structured fields, so quota and generic build
// failures both keep IsError false and let the frontend detect failures via
// the "error" key.
func buildFailureToolResponse(
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
	action buildFailureAction,
	buildID uuid.UUID,
	err error,
) fantasy.ToolResponse {
	msg := err.Error()
	if codersdk.JobIsInsufficientQuotaErrorCode(buildErrorCode(err)) {
		return quotaErrorToolResponse(
			ctx,
			logger,
			db,
			ownerID,
			organizationID,
			msg,
			buildID,
			action,
		)
	}
	return buildToolResponse(newBuildError(msg, buildID))
}
