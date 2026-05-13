package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// GroupAIBudget is the AI spend limit configured for a group. The limit is
// per-user: every member of the group has their own running spend tracked
// against this cap.
type GroupAIBudget struct {
	GroupID          uuid.UUID `json:"group_id" format:"uuid"`
	SpendLimitMicros int64     `json:"spend_limit_micros"`
	CreatedAt        time.Time `json:"created_at" format:"date-time"`
	UpdatedAt        time.Time `json:"updated_at" format:"date-time"`
}

// UpsertGroupAIBudgetRequest is the body for creating or updating a group's
// AI budget. SpendLimitMicros must be greater than zero.
type UpsertGroupAIBudgetRequest struct {
	SpendLimitMicros int64 `json:"spend_limit_micros" validate:"required,gt=0"`
}

func (c *Client) GroupAIBudget(ctx context.Context, group uuid.UUID) (GroupAIBudget, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/groups/%s/ai/budget", group.String()),
		nil,
	)
	if err != nil {
		return GroupAIBudget{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GroupAIBudget{}, ReadBodyAsError(res)
	}
	var resp GroupAIBudget
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) UpsertGroupAIBudget(ctx context.Context, group uuid.UUID, req UpsertGroupAIBudgetRequest) (GroupAIBudget, error) {
	res, err := c.Request(ctx, http.MethodPut,
		fmt.Sprintf("/api/v2/groups/%s/ai/budget", group.String()),
		req,
	)
	if err != nil {
		return GroupAIBudget{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GroupAIBudget{}, ReadBodyAsError(res)
	}
	var resp GroupAIBudget
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) DeleteGroupAIBudget(ctx context.Context, group uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/groups/%s/ai/budget", group.String()),
		nil,
	)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
