package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type AIBridgeListInterceptionsRequest struct {
	PeriodStart time.Time                       `json:"period_start" format:"date-time"`
	PeriodEnd   time.Time                       `json:"period_end" format:"date-time"`
	InitiatorID uuid.UUID                       `json:"initiator_id" format:"uuid"`
	Limit       int32                           `json:"limit"`
	Cursor      AIBridgeListInterceptionsCursor `json:"cursor"`
}

type AIBridgeListInterceptionsTokens struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
}

type AIBridgeListInterceptionsTool struct {
	Server string `json:"server"`
	Tool   string `json:"tool"`
	Input  string `json:"args"`
}

type AIBridgeListInterceptionsResult struct {
	InterceptionID uuid.UUID                       `json:"interception_id" format:"uuid"`
	UserID         uuid.UUID                       `json:"user_id" format:"uuid"`
	Provider       string                          `json:"provider"`
	Model          string                          `json:"model"`
	Prompt         string                          `json:"prompt"`
	StartedAt      time.Time                       `json:"started_at" format:"date-time"`
	Tokens         AIBridgeListInterceptionsTokens `json:"tokens"`
	Tools          []AIBridgeListInterceptionsTool `json:"tools"`
}

type AIBridgeListInterceptionsCursor struct {
	ID   uuid.UUID `json:"id" format:"uuid"`
	Time time.Time `json:"time" format:"date-time"`
}

type AIBridgeListInterceptionsResponse struct {
	Results []AIBridgeListInterceptionsResult `json:"results"`
	Cursor  AIBridgeListInterceptionsCursor   `json:"cursor"`
}

// AIBridgeListInterceptions returns AIBridge interceptions filtered by parameters.
func (c *ExperimentalClient) AIBridgeListInterceptions(ctx context.Context, params AIBridgeListInterceptionsRequest) (AIBridgeListInterceptionsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/aibridge/interceptions", params)
	if err != nil {
		return AIBridgeListInterceptionsResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIBridgeListInterceptionsResponse{}, ReadBodyAsError(res)
	}
	var resp AIBridgeListInterceptionsResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
