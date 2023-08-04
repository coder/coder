package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// InsightsReportInterval enums.
const (
	InsightsReportIntervalDay InsightsReportInterval = "day"
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
	var qp []string
	qp = append(qp, fmt.Sprintf("start_time=%s", req.StartTime.Format(insightsTimeLayout)))
	qp = append(qp, fmt.Sprintf("end_time=%s", req.EndTime.Format(insightsTimeLayout)))
	if len(req.TemplateIDs) > 0 {
		var templateIDs []string
		for _, id := range req.TemplateIDs {
			templateIDs = append(templateIDs, id.String())
		}
		qp = append(qp, fmt.Sprintf("template_ids=%s", strings.Join(templateIDs, ",")))
	}

	reqURL := fmt.Sprintf("/api/v2/insights/user-latency?%s", strings.Join(qp, "&"))
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

// TemplateInsightsResponse is the response from the template insights endpoint.
type TemplateInsightsResponse struct {
	Report          TemplateInsightsReport           `json:"report"`
	IntervalReports []TemplateInsightsIntervalReport `json:"interval_reports"`
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
	Interval    InsightsReportInterval `json:"interval"`
	ActiveUsers int64                  `json:"active_users" example:"14"`
}

// TemplateAppsType defines the type of app reported.
type TemplateAppsType string

// TemplateAppsType enums.
const (
	TemplateAppsTypeBuiltin TemplateAppsType = "builtin"
	// TODO(mafredri): To be introduced in a future pull request.
	// TemplateAppsTypeApp     TemplateAppsType = "app"
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
	StartTime   time.Time              `json:"start_time" format:"date-time"`
	EndTime     time.Time              `json:"end_time" format:"date-time"`
	TemplateIDs []uuid.UUID            `json:"template_ids" format:"uuid"`
	Interval    InsightsReportInterval `json:"interval"`
}

func (c *Client) TemplateInsights(ctx context.Context, req TemplateInsightsRequest) (TemplateInsightsResponse, error) {
	var qp []string
	qp = append(qp, fmt.Sprintf("start_time=%s", req.StartTime.Format(insightsTimeLayout)))
	qp = append(qp, fmt.Sprintf("end_time=%s", req.EndTime.Format(insightsTimeLayout)))
	if len(req.TemplateIDs) > 0 {
		var templateIDs []string
		for _, id := range req.TemplateIDs {
			templateIDs = append(templateIDs, id.String())
		}
		qp = append(qp, fmt.Sprintf("template_ids=%s", strings.Join(templateIDs, ",")))
	}
	if req.Interval != "" {
		qp = append(qp, fmt.Sprintf("interval=%s", req.Interval))
	}

	reqURL := fmt.Sprintf("/api/v2/insights/templates?%s", strings.Join(qp, "&"))
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
