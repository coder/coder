package chattool

import (
	"context"
	"encoding/json"
	"errors"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

const workspaceQuotaErrorTitle = "Workspace quota reached"

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

type quotaErrorResult struct {
	ErrorCode codersdk.JobErrorCode `json:"error_code"`
	Error     string                `json:"error"`
	Title     string                `json:"title"`
	Message   string                `json:"message"`
	NextSteps []string              `json:"next_steps"`
	BuildID   string                `json:"build_id,omitempty"`
	Quota     *quotaErrorDetails    `json:"quota,omitempty"`
}

type quotaErrorDetails struct {
	CreditsConsumed int64 `json:"credits_consumed"`
	Budget          int64 `json:"budget"`
}

func quotaToolResponse(r quotaErrorResult) fantasy.ToolResponse {
	data, err := json.Marshal(r)
	if err != nil {
		return fantasy.NewTextResponse("{}")
	}
	return fantasy.NewTextResponse(string(data))
}

func newQuotaError(
	msg string,
	buildID uuid.UUID,
	action string,
	quota *quotaErrorDetails,
) quotaErrorResult {
	message := "Coder could not create this workspace because your workspace quota is full."
	if action == "start" {
		message = "Coder could not start this workspace because your workspace quota is full."
	}

	r := quotaErrorResult{
		ErrorCode: codersdk.JobErrorCodeInsufficientQuota,
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
	db database.Store,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
) (*quotaErrorDetails, error) {
	if db == nil || ownerID == uuid.Nil || organizationID == uuid.Nil {
		return nil, nil
	}

	consumed, err := db.GetQuotaConsumedForUser(ctx, database.GetQuotaConsumedForUserParams{
		OwnerID:        ownerID,
		OrganizationID: organizationID,
	})
	if err != nil {
		return nil, err
	}
	budget, err := db.GetQuotaAllowanceForUser(ctx, database.GetQuotaAllowanceForUserParams{
		UserID:         ownerID,
		OrganizationID: organizationID,
	})
	if err != nil {
		return nil, err
	}
	return &quotaErrorDetails{
		CreditsConsumed: consumed,
		Budget:          budget,
	}, nil
}

func quotaErrorToolResponse(
	ctx context.Context,
	db database.Store,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
	msg string,
	buildID uuid.UUID,
	action string,
) fantasy.ToolResponse {
	quota, _ := workspaceQuotaDetails(ctx, db, ownerID, organizationID)
	return quotaToolResponse(newQuotaError(msg, buildID, action, quota))
}
