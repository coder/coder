package codersdk

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"golang.org/x/xerrors"
)

// AgentTimeInterval groups UTC calendar dates into chart buckets.
type AgentTimeInterval string

// Agent time chart intervals use Monday-start weeks and calendar months.
const (
	AgentTimeIntervalDay   AgentTimeInterval = "day"
	AgentTimeIntervalWeek  AgentTimeInterval = "week"
	AgentTimeIntervalMonth AgentTimeInterval = "month"
)

// AgentTimeRequest selects a half-open UTC date range. Omitting StartDate
// includes all available recorded history. Omitting EndDate includes today.
type AgentTimeRequest struct {
	StartDate      string            `json:"start_date,omitempty" format:"date"`
	EndDate        string            `json:"end_date,omitempty" format:"date"`
	Interval       AgentTimeInterval `json:"interval,omitempty"`
	OrganizationID string            `json:"organization_id,omitempty" format:"uuid"`
	UserID         string            `json:"user_id,omitempty" format:"uuid"`
	GroupBy        string            `json:"group_by,omitempty" enums:"organization,user"`
	Limit          int               `json:"limit,omitempty"`
	Offset         int               `json:"offset,omitempty"`
	SortBy         string            `json:"sort_by,omitempty" enums:"agent_time,name"`
	SortOrder      string            `json:"sort_order,omitempty" enums:"asc,desc"`
}

// AgentTimeReport contains recorded time, not workspace uptime or elapsed
// execution intervals. Decimal millisecond strings preserve integer precision
// for JavaScript clients, including sums larger than a signed 64-bit integer.
type AgentTimeReport struct {
	StartDate        string               `json:"start_date" format:"date"`
	EndDate          string               `json:"end_date" format:"date"`
	Interval         AgentTimeInterval    `json:"interval"`
	TotalAgentTimeMS string               `json:"total_agent_time_ms"`
	Buckets          []AgentTimeBucket    `json:"buckets"`
	Rows             []AgentTimeBreakdown `json:"rows"`
	Count            int64                `json:"count"`
	Status           AgentTimeStatus      `json:"status"`
	HistoricalNotice string               `json:"historical_notice"`
}

// AgentTimeBucket describes a UTC calendar bucket clipped to the requested
// range. A null total means no recorded data and unknown accounting coverage.
type AgentTimeBucket struct {
	StartDate   string  `json:"start_date" format:"date"`
	EndDate     string  `json:"end_date" format:"date"`
	AgentTimeMS *string `json:"agent_time_ms"`
	Partial     bool    `json:"partial"`
	Complete    bool    `json:"complete"`
}

// AgentTimeBreakdown identifies an organization or user even after deletion.
type AgentTimeBreakdown struct {
	ID          string `json:"id" format:"uuid"`
	Name        string `json:"name"`
	Deleted     bool   `json:"deleted"`
	AgentTimeMS string `json:"agent_time_ms"`
}

// AgentTimeStatus describes capture and recoverable-history backfill. A
// completed backfill cannot reconstruct previously deleted or unrecorded time.
type AgentTimeStatus struct {
	CaptureStartedAt  string  `json:"capture_started_at" format:"date-time"`
	BackfillComplete  bool    `json:"backfill_complete"`
	BackfillError     string  `json:"backfill_error"`
	ProcessedMessages string  `json:"processed_messages"`
	EarliestDate      *string `json:"earliest_date" format:"date"`
}

// AgentTime returns a deployment administrator's recorded agent time report.
func (c *Client) AgentTime(ctx context.Context, req AgentTimeRequest) (AgentTimeReport, error) {
	return c.requestAgentTime(ctx, "/api/v2/agent-time", req)
}

// OrganizationAgentTime returns a report restricted to one organization's
// recorded history. Deployment-config read permission is required.
func (c *Client) OrganizationAgentTime(ctx context.Context, organization string, req AgentTimeRequest) (AgentTimeReport, error) {
	return c.requestAgentTime(ctx, "/api/v2/organizations/"+url.PathEscape(organization)+"/agent-time", req)
}

func (c *Client) requestAgentTime(ctx context.Context, path string, req AgentTimeRequest) (AgentTimeReport, error) {
	q := url.Values{}
	for key, value := range map[string]string{
		"start_date": req.StartDate, "end_date": req.EndDate,
		"interval": string(req.Interval), "organization_id": req.OrganizationID,
		"user_id": req.UserID, "group_by": req.GroupBy,
		"sort_by": req.SortBy, "sort_order": req.SortOrder,
	} {
		if value != "" {
			q.Set(key, value)
		}
	}
	if req.Limit != 0 {
		q.Set("limit", strconv.Itoa(req.Limit))
	}
	if req.Offset != 0 {
		q.Set("offset", strconv.Itoa(req.Offset))
	}
	resp, err := c.Request(ctx, http.MethodGet, path+"?"+q.Encode(), nil)
	if err != nil {
		return AgentTimeReport{}, xerrors.Errorf("request agent time: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return AgentTimeReport{}, ReadBodyAsError(resp)
	}
	var report AgentTimeReport
	return report, ReadBodyAsJSON(resp, &report)
}
