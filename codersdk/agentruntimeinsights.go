package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// AgentRuntimeInsightsResponse is the response from the agent runtime
// insights endpoint. It backs the Coder Agents usage dashboard's summary
// figure and chart.
type AgentRuntimeInsightsResponse struct {
	StartTime time.Time                 `json:"start_time" format:"date-time"`
	EndTime   time.Time                 `json:"end_time" format:"date-time"`
	TotalMs   int64                     `json:"total_ms" example:"3600000"`
	ByDay     []AgentRuntimeInsightsDay `json:"by_day"`
}

// AgentRuntimeInsightsDay is a single day's total agent runtime.
type AgentRuntimeInsightsDay struct {
	Day     time.Time `json:"day" format:"date"`
	TotalMs int64     `json:"total_ms" example:"3600000"`
}

// AgentRuntimeInsightsByUserResponse is a page of per-user agent runtime
// totals, ordered by total runtime descending.
type AgentRuntimeInsightsByUserResponse struct {
	Users []AgentRuntimeInsightsUser `json:"users"`
	// Count is the total number of users with agent runtime in the
	// requested range, for pagination.
	Count int64 `json:"count"`
}

// AgentRuntimeInsightsUser is a single user's agent runtime total.
type AgentRuntimeInsightsUser struct {
	UserID       uuid.UUID `json:"user_id" format:"uuid"`
	Username     string    `json:"username"`
	AvatarURL    string    `json:"avatar_url" format:"uri"`
	TotalMs      int64     `json:"total_ms" example:"3600000"`
	MessageCount int64     `json:"message_count" example:"120"`
}

// AgentRuntimeInsightsRequest is the request for the agent runtime insights
// summary and chart endpoint.
type AgentRuntimeInsightsRequest struct {
	StartTime time.Time `json:"start_time" format:"date-time"`
	EndTime   time.Time `json:"end_time" format:"date-time"`
}

// AgentRuntimeInsights returns the total agent runtime for the deployment
// within the given range, bucketed by day.
func (c *Client) AgentRuntimeInsights(ctx context.Context, req AgentRuntimeInsightsRequest) (AgentRuntimeInsightsResponse, error) {
	qp := url.Values{}
	qp.Add("start_time", req.StartTime.Format(insightsTimeLayout))
	qp.Add("end_time", req.EndTime.Format(insightsTimeLayout))

	reqURL := fmt.Sprintf("/api/experimental/chats/agent-runtime-insights?%s", qp.Encode())
	resp, err := c.Request(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return AgentRuntimeInsightsResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return AgentRuntimeInsightsResponse{}, ReadBodyAsError(resp)
	}
	var result AgentRuntimeInsightsResponse
	return result, ReadBodyAsJSON(resp, &result)
}

// AgentRuntimeInsightsByUserRequest is the request for the paginated,
// per-user agent runtime insights endpoint.
type AgentRuntimeInsightsByUserRequest struct {
	StartTime time.Time `json:"start_time" format:"date-time"`
	EndTime   time.Time `json:"end_time" format:"date-time"`
	Pagination
}

// AgentRuntimeInsightsByUser returns a page of per-user agent runtime totals
// within the given range, ordered by total runtime descending.
func (c *Client) AgentRuntimeInsightsByUser(ctx context.Context, req AgentRuntimeInsightsByUserRequest) (AgentRuntimeInsightsByUserResponse, error) {
	qp := url.Values{}
	qp.Add("start_time", req.StartTime.Format(insightsTimeLayout))
	qp.Add("end_time", req.EndTime.Format(insightsTimeLayout))
	if req.Limit > 0 {
		qp.Add("limit", strconv.Itoa(req.Limit))
	}
	if req.Offset > 0 {
		qp.Add("offset", strconv.Itoa(req.Offset))
	}

	reqURL := fmt.Sprintf("/api/experimental/chats/agent-runtime-insights/users?%s", qp.Encode())
	resp, err := c.Request(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return AgentRuntimeInsightsByUserResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return AgentRuntimeInsightsByUserResponse{}, ReadBodyAsError(resp)
	}
	var result AgentRuntimeInsightsByUserResponse
	return result, ReadBodyAsJSON(resp, &result)
}
