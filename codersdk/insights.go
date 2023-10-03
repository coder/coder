package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// Duplicated in coderd.
const insightsTimeLayout = time.RFC3339

// InsightsReportInterval is the interval of time over which to generate a
// smaller insights report within a time range.
type InsightsReportInterval string

// Days returns the duration of the interval in days.
func (interval InsightsReportInterval) Days() int32 {
	switch interval {
	case InsightsReportIntervalDay:
		return 1
	case InsightsReportIntervalWeek:
		return 7
	default:
		panic("developer error: unsupported report interval")
	}
}

// InsightsReportInterval enums.
const (
	InsightsReportIntervalDay  InsightsReportInterval = "day"
	InsightsReportIntervalWeek InsightsReportInterval = "week"
)

// TemplateInsightsSection defines the section to be included in the template insights response.
type TemplateInsightsSection string

// TemplateInsightsSection enums.
const (
	TemplateInsightsSectionIntervalReports TemplateInsightsSection = "interval_reports"
	TemplateInsightsSectionReport          TemplateInsightsSection = "report"
)

// UserLatencyInsightsResponse is the response from the user latency insights
// endpoint.
type UserLatencyInsightsResponse struct {
	Report UserLatencyInsightsReport `json:"report"`
}

// UserLatencyInsightsReport is the report from the user latency insights
// endpoint.
type UserLatencyInsightsReport struct {
	StartTime   time.Time     `json:"start_time" format:"date-time"`
	EndTime     time.Time     `json:"end_time" format:"date-time"`
	TemplateIDs []uuid.UUID   `json:"template_ids" format:"uuid"`
	Users       []UserLatency `json:"users"`
}

// UserLatency shows the connection latency for a user.
type UserLatency struct {
	TemplateIDs []uuid.UUID       `json:"template_ids" format:"uuid"`
	UserID      uuid.UUID         `json:"user_id" format:"uuid"`
	Username    string            `json:"username"`
	AvatarURL   string            `json:"avatar_url" format:"uri"`
	LatencyMS   ConnectionLatency `json:"latency_ms"`
}

// UserActivityInsightsResponse is the response from the user activity insights
// endpoint.
type UserActivityInsightsResponse struct {
	Report UserActivityInsightsReport `json:"report"`
}

// UserActivityInsightsReport is the report from the user activity insights
// endpoint.
type UserActivityInsightsReport struct {
	StartTime   time.Time      `json:"start_time" format:"date-time"`
	EndTime     time.Time      `json:"end_time" format:"date-time"`
	TemplateIDs []uuid.UUID    `json:"template_ids" format:"uuid"`
	Users       []UserActivity `json:"users"`
}

// UserActivity shows the session time for a user.
type UserActivity struct {
	TemplateIDs []uuid.UUID `json:"template_ids" format:"uuid"`
	UserID      uuid.UUID   `json:"user_id" format:"uuid"`
	Username    string      `json:"username"`
	AvatarURL   string      `json:"avatar_url" format:"uri"`
	Seconds     int64       `json:"seconds" example:"80500"`
}

// ConnectionLatency shows the latency for a connection.
type ConnectionLatency struct {
	P50 float64 `json:"p50" example:"31.312"`
	P95 float64 `json:"p95" example:"119.832"`
}

type UserLatencyInsightsRequest struct {
	StartTime   time.Time   `json:"start_time" format:"date-time"`
	EndTime     time.Time   `json:"end_time" format:"date-time"`
	TemplateIDs []uuid.UUID `json:"template_ids" format:"uuid"`
}

func (c *Client) UserLatencyInsights(ctx context.Context, req UserLatencyInsightsRequest) (UserLatencyInsightsResponse, error) {
	qp := url.Values{}
	qp.Add("start_time", req.StartTime.Format(insightsTimeLayout))
	qp.Add("end_time", req.EndTime.Format(insightsTimeLayout))
	if len(req.TemplateIDs) > 0 {
		var templateIDs []string
		for _, id := range req.TemplateIDs {
			templateIDs = append(templateIDs, id.String())
		}
		qp.Add("template_ids", strings.Join(templateIDs, ","))
	}

	reqURL := fmt.Sprintf("/api/v2/insights/user-latency?%s", qp.Encode())
	resp, err := c.Request(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return UserLatencyInsightsResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return UserLatencyInsightsResponse{}, ReadBodyAsError(resp)
	}
	var result UserLatencyInsightsResponse
	return result, json.NewDecoder(resp.Body).Decode(&result)
}

type UserActivityInsightsRequest struct {
	StartTime   time.Time   `json:"start_time" format:"date-time"`
	EndTime     time.Time   `json:"end_time" format:"date-time"`
	TemplateIDs []uuid.UUID `json:"template_ids" format:"uuid"`
}

func (c *Client) UserActivityInsights(ctx context.Context, req UserActivityInsightsRequest) (UserActivityInsightsResponse, error) {
	qp := url.Values{}
	qp.Add("start_time", req.StartTime.Format(insightsTimeLayout))
	qp.Add("end_time", req.EndTime.Format(insightsTimeLayout))
	if len(req.TemplateIDs) > 0 {
		var templateIDs []string
		for _, id := range req.TemplateIDs {
			templateIDs = append(templateIDs, id.String())
		}
		qp.Add("template_ids", strings.Join(templateIDs, ","))
	}

	reqURL := fmt.Sprintf("/api/v2/insights/user-activity?%s", qp.Encode())
	resp, err := c.Request(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return UserActivityInsightsResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return UserActivityInsightsResponse{}, ReadBodyAsError(resp)
	}
	var result UserActivityInsightsResponse
	return result, json.NewDecoder(resp.Body).Decode(&result)
}

// TemplateInsightsResponse is the response from the template insights endpoint.
type TemplateInsightsResponse struct {
	Report          *TemplateInsightsReport          `json:"report,omitempty"`
	IntervalReports []TemplateInsightsIntervalReport `json:"interval_reports,omitempty"`
}

// TemplateInsightsReport is the report from the template insights endpoint.
type TemplateInsightsReport struct {
	StartTime       time.Time                `json:"start_time" format:"date-time"`
	EndTime         time.Time                `json:"end_time" format:"date-time"`
	TemplateIDs     []uuid.UUID              `json:"template_ids" format:"uuid"`
	ActiveUsers     int64                    `json:"active_users" example:"22"`
	AppsUsage       []TemplateAppUsage       `json:"apps_usage"`
	ParametersUsage []TemplateParameterUsage `json:"parameters_usage"`
}

// TemplateInsightsIntervalReport is the report from the template insights
// endpoint for a specific interval.
type TemplateInsightsIntervalReport struct {
	StartTime   time.Time              `json:"start_time" format:"date-time"`
	EndTime     time.Time              `json:"end_time" format:"date-time"`
	TemplateIDs []uuid.UUID            `json:"template_ids" format:"uuid"`
	Interval    InsightsReportInterval `json:"interval" example:"week"`
	ActiveUsers int64                  `json:"active_users" example:"14"`
}

// TemplateAppsType defines the type of app reported.
type TemplateAppsType string

// TemplateAppsType enums.
const (
	TemplateAppsTypeBuiltin TemplateAppsType = "builtin"
	TemplateAppsTypeApp     TemplateAppsType = "app"
)

// TemplateAppUsage shows the usage of an app for one or more templates.
type TemplateAppUsage struct {
	TemplateIDs []uuid.UUID      `json:"template_ids" format:"uuid"`
	Type        TemplateAppsType `json:"type" example:"builtin"`
	DisplayName string           `json:"display_name" example:"Visual Studio Code"`
	Slug        string           `json:"slug" example:"vscode"`
	Icon        string           `json:"icon"`
	Seconds     int64            `json:"seconds" example:"80500"`
}

// TemplateParameterUsage shows the usage of a parameter for one or more
// templates.
type TemplateParameterUsage struct {
	TemplateIDs []uuid.UUID                      `json:"template_ids" format:"uuid"`
	DisplayName string                           `json:"display_name"`
	Name        string                           `json:"name"`
	Type        string                           `json:"type"`
	Description string                           `json:"description"`
	Options     []TemplateVersionParameterOption `json:"options,omitempty"`
	Values      []TemplateParameterValue         `json:"values"`
}

// TemplateParameterValue shows the usage of a parameter value for one or more
// templates.
type TemplateParameterValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

type TemplateInsightsRequest struct {
	StartTime   time.Time                 `json:"start_time" format:"date-time"`
	EndTime     time.Time                 `json:"end_time" format:"date-time"`
	TemplateIDs []uuid.UUID               `json:"template_ids" format:"uuid"`
	Interval    InsightsReportInterval    `json:"interval" example:"day"`
	Sections    []TemplateInsightsSection `json:"sections" example:"report"`
}

func (c *Client) TemplateInsights(ctx context.Context, req TemplateInsightsRequest) (TemplateInsightsResponse, error) {
	qp := url.Values{}
	qp.Add("start_time", req.StartTime.Format(insightsTimeLayout))
	qp.Add("end_time", req.EndTime.Format(insightsTimeLayout))
	if len(req.TemplateIDs) > 0 {
		var templateIDs []string
		for _, id := range req.TemplateIDs {
			templateIDs = append(templateIDs, id.String())
		}
		qp.Add("template_ids", strings.Join(templateIDs, ","))
	}
	if req.Interval != "" {
		qp.Add("interval", string(req.Interval))
	}
	if len(req.Sections) > 0 {
		var sections []string
		for _, sec := range req.Sections {
			sections = append(sections, string(sec))
		}
		qp.Add("sections", strings.Join(sections, ","))
	}

	reqURL := fmt.Sprintf("/api/v2/insights/templates?%s", qp.Encode())
	resp, err := c.Request(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return TemplateInsightsResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TemplateInsightsResponse{}, ReadBodyAsError(resp)
	}
	var result TemplateInsightsResponse
	return result, json.NewDecoder(resp.Body).Decode(&result)
}
