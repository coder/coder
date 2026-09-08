package coderd

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

const agentTimeHistoricalNotice = "Agent time includes recorded message contributions only, attributed to their UTC creation date. Previously deleted messages and time that was never recorded cannot be recovered. Historical totals may be incomplete even after backfill finishes. Null recorded time does not mean no work occurred."

type agentTimeQuery struct {
	start, end                 time.Time
	organizationID, userID     uuid.UUID
	interval                   codersdk.AgentTimeInterval
	groupBy, sortBy, sortOrder string
	limit, offset              int32
}

func parseAgentTimeQuery(values url.Values, organization string, now time.Time) (agentTimeQuery, []codersdk.ValidationError) {
	p := httpapi.NewQueryParamParser()
	date := func(value string) (time.Time, error) {
		parsed, err := time.Parse(time.DateOnly, value)
		if err != nil || parsed.Year() < 1 {
			return time.Time{}, xerrors.New("must be a UTC calendar date in YYYY-MM-DD format")
		}
		return parsed, nil
	}
	q := agentTimeQuery{
		start:          httpapi.ParseCustom(p, values, time.Time{}, "start_date", date),
		end:            httpapi.ParseCustom(p, values, now.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1), "end_date", date),
		organizationID: p.UUID(values, uuid.Nil, "organization_id"),
		userID:         p.UUID(values, uuid.Nil, "user_id"),
		interval:       codersdk.AgentTimeInterval(p.String(values, "day", "interval")),
		groupBy:        p.String(values, "organization", "group_by"),
		sortBy:         p.String(values, "agent_time", "sort_by"),
		sortOrder:      p.String(values, "desc", "sort_order"),
	}
	limit, offset := p.Int(values, 25, "limit"), p.Int(values, 0, "offset")
	addError := func(field, detail string) {
		p.Errors = append(p.Errors, codersdk.ValidationError{Field: field, Detail: detail})
	}
	if limit < 1 || limit > 100 {
		addError("limit", "must be between 1 and 100")
	} else {
		q.limit = int32(limit)
	}
	if offset < 0 || offset > math.MaxInt32 {
		addError("offset", "must be between 0 and 2147483647")
	} else {
		q.offset = int32(offset)
	}
	if q.interval != codersdk.AgentTimeIntervalDay && q.interval != codersdk.AgentTimeIntervalWeek && q.interval != codersdk.AgentTimeIntervalMonth {
		addError("interval", "must be day, week, or month")
	}
	if q.groupBy != "organization" && q.groupBy != "user" {
		addError("group_by", "must be organization or user")
	}
	if q.sortBy != "agent_time" && q.sortBy != "name" {
		addError("sort_by", "must be agent_time or name")
	}
	if q.sortOrder != "asc" && q.sortOrder != "desc" {
		addError("sort_order", "must be asc or desc")
	}
	if q.end.After(now.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)) {
		addError("end_date", "must not be later than tomorrow UTC")
	}
	if !q.start.IsZero() && !q.start.Before(q.end) {
		addError("end_date", "must be after start_date")
	}
	for key, id := range map[string]uuid.UUID{"organization_id": q.organizationID, "user_id": q.userID} {
		if values.Get(key) != "" && id == uuid.Nil {
			addError(key, "must be a nonzero UUID")
		}
	}
	if organization != "" {
		id, err := uuid.Parse(organization)
		switch {
		case err != nil || id == uuid.Nil:
			addError("organization", "must be a nonzero organization UUID")
		case q.organizationID != uuid.Nil && q.organizationID != id:
			addError("organization_id", "must match the organization in the route")
		default:
			q.organizationID = id
		}
	}
	p.ErrorExcessParams(values)
	return q, p.Errors
}

type agentTimeRangeError struct{ message string }

func (e *agentTimeRangeError) Error() string { return e.message }

func readAgentTimeReport(ctx context.Context, db database.Store, q agentTimeQuery, now time.Time) (codersdk.AgentTimeReport, error) {
	report := codersdk.AgentTimeReport{Interval: q.interval, Rows: []codersdk.AgentTimeBreakdown{}, HistoricalNotice: agentTimeHistoricalNotice}
	earliest, err := db.GetAgentTimeEarliestDate(ctx, database.GetAgentTimeEarliestDateParams{
		OrganizationID:         q.organizationID,
		UserID:                 q.userID,
		UseOrganizationSummary: q.userID == uuid.Nil,
	})
	if err != nil {
		return report, xerrors.Errorf("read earliest agent time date: %w", err)
	}
	if earliest != "" {
		report.Status.EarliestDate = &earliest
	}
	if q.start.IsZero() {
		q.start = q.end.AddDate(0, 0, -1)
		if earliest != "" {
			q.start, err = time.Parse(time.DateOnly, earliest)
			if err != nil {
				return report, xerrors.Errorf("parse earliest agent time date: %w", err)
			}
		}
	}
	if !q.start.Before(q.end) {
		return report, &agentTimeRangeError{message: "end_date must be after the first available date or start_date."}
	}
	// Validate the point budget before issuing the chart aggregation query.
	buckets, err := agentTimeBuckets(q.start, q.end, q.interval, now)
	if err != nil {
		return report, err
	}
	report.StartDate = q.start.Format(time.DateOnly)
	report.EndDate = q.end.Format(time.DateOnly)
	report.Buckets = buckets
	summary, err := db.GetAgentTimeSummary(ctx, database.GetAgentTimeSummaryParams{
		StartDate:              q.start,
		EndDate:                q.end,
		OrganizationID:         q.organizationID,
		UserID:                 q.userID,
		GroupBy:                q.groupBy,
		UseOrganizationSummary: q.groupBy == "organization" && q.userID == uuid.Nil,
	})
	if err != nil {
		return report, xerrors.Errorf("read agent time summary: %w", err)
	}
	report.TotalAgentTimeMS = summary.AgentTimeMs
	report.Count = summary.Count
	rows, err := db.GetAgentTimeBreakdown(ctx, database.GetAgentTimeBreakdownParams{
		StartDate:              q.start,
		EndDate:                q.end,
		OrganizationID:         q.organizationID,
		UserID:                 q.userID,
		GroupBy:                q.groupBy,
		PageLimit:              q.limit,
		PageOffset:             q.offset,
		SortBy:                 q.sortBy,
		SortOrder:              q.sortOrder,
		UseOrganizationSummary: q.groupBy == "organization" && q.userID == uuid.Nil,
	})
	if err != nil {
		return report, xerrors.Errorf("read agent time breakdown: %w", err)
	}
	for _, row := range rows {
		report.Rows = append(report.Rows, agentTimeBreakdownToSDK(row))
	}
	points, err := db.GetAgentTimeBuckets(ctx, database.GetAgentTimeBucketsParams{
		StartDate:              q.start,
		EndDate:                q.end,
		OrganizationID:         q.organizationID,
		UserID:                 q.userID,
		Interval:               string(q.interval),
		UseOrganizationSummary: q.userID == uuid.Nil,
	})
	if err != nil {
		return report, xerrors.Errorf("read agent time buckets: %w", err)
	}
	status, err := readAgentTimeStatus(ctx, db, q.organizationID)
	if err != nil {
		return report, err
	}
	status.EarliestDate = report.Status.EarliestDate
	report.Status = status
	captured, err := time.Parse(time.RFC3339Nano, status.CaptureStartedAt)
	if err != nil {
		return report, xerrors.Errorf("parse agent time capture date: %w", err)
	}
	coverageStart := captured.UTC().Truncate(24 * time.Hour)
	if coverageStart.Before(captured) {
		coverageStart = coverageStart.AddDate(0, 0, 1)
	}
	totals := make(map[string]string, len(points))
	for _, point := range points {
		totals[point.BucketDate] = point.AgentTimeMs
	}
	for i := range report.Buckets {
		bucket := &report.Buckets[i]
		start, _ := time.Parse(time.DateOnly, bucket.StartDate)
		bucket.Complete = status.BackfillComplete && !start.Before(coverageStart)
		value, ok := totals[agentTimeBucketStart(start, q.interval).Format(time.DateOnly)]
		if ok {
			bucket.AgentTimeMS = &value
		} else if bucket.Complete {
			bucket.AgentTimeMS = new("0")
		}
	}
	return report, nil
}

func readAgentTimeStatus(ctx context.Context, db database.Store, organizationID uuid.UUID) (codersdk.AgentTimeStatus, error) {
	status, err := db.GetAgentTimeStatus(ctx, organizationID)
	if err != nil {
		return codersdk.AgentTimeStatus{}, xerrors.Errorf("read agent time accounting status: %w", err)
	}
	return codersdk.AgentTimeStatus{
		CaptureStartedAt:  status.CaptureStartedAt.UTC().Format(time.RFC3339Nano),
		BackfillComplete:  status.BackfillComplete,
		BackfillError:     status.BackfillError,
		ProcessedMessages: fmt.Sprint(status.ProcessedMessages),
	}, nil
}

func agentTimeBreakdownToSDK(row database.GetAgentTimeBreakdownRow) codersdk.AgentTimeBreakdown {
	return codersdk.AgentTimeBreakdown{ID: row.ID.String(), Name: row.Name, Deleted: row.Deleted, AgentTimeMS: row.AgentTimeMs}
}

func agentTimeBucketStart(date time.Time, interval codersdk.AgentTimeInterval) time.Time {
	switch interval {
	case codersdk.AgentTimeIntervalWeek:
		return date.AddDate(0, 0, -(int(date.Weekday())+6)%7)
	case codersdk.AgentTimeIntervalMonth:
		return time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return date
	}
}

func agentTimeBuckets(start, end time.Time, interval codersdk.AgentTimeInterval, now time.Time) ([]codersdk.AgentTimeBucket, error) {
	buckets := make([]codersdk.AgentTimeBucket, 0)
	for date := agentTimeBucketStart(start, interval); date.Before(end); {
		if len(buckets) == 1000 {
			return nil, &agentTimeRangeError{message: "The range exceeds 1000 chart points. Select a wider interval."}
		}
		var next time.Time
		switch interval {
		case codersdk.AgentTimeIntervalDay:
			next = date.AddDate(0, 0, 1)
		case codersdk.AgentTimeIntervalWeek:
			next = date.AddDate(0, 0, 7)
		case codersdk.AgentTimeIntervalMonth:
			next = date.AddDate(0, 1, 0)
		default:
			return nil, &agentTimeRangeError{message: fmt.Sprintf("Invalid agent time interval %q.", interval)}
		}
		lower, upper := date, next
		if lower.Before(start) {
			lower = start
		}
		if upper.After(end) {
			upper = end
		}
		buckets = append(buckets, codersdk.AgentTimeBucket{StartDate: lower.Format(time.DateOnly), EndDate: upper.Format(time.DateOnly), Partial: !lower.Equal(date) || !upper.Equal(next) || upper.After(now)})
		date = next
	}
	return buckets, nil
}
