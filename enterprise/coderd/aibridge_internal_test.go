package coderd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agplcoderd "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

// Use fixed dates to keep every case deterministic.
var (
	aiSpendPeriodNow        = time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
	aiSpendPeriodMonthStart = time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	aiSpendPeriodMonthEnd   = time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
)

// aiSpendPeriodCase is a request to a period helper and its expected result.
type aiSpendPeriodCase struct {
	name      string
	retention time.Duration
	params    map[string]string
	// wantMessage is the expected error message. Empty means the request
	// succeeds with want.
	wantMessage string
	want        aiSpendPeriod
}

// newAISpendPeriodTestAPI returns an API with a mock clock at
// aiSpendPeriodNow and the given AI Gateway data retention.
func newAISpendPeriodTestAPI(t *testing.T, retention time.Duration) *API {
	t.Helper()
	clock := quartz.NewMock(t)
	clock.Set(aiSpendPeriodNow)
	deploymentValues := &codersdk.DeploymentValues{}
	deploymentValues.AI.BridgeConfig.Retention = serpent.Duration(retention)
	return &API{Options: &Options{Options: &agplcoderd.Options{
		Logger:           slogtest.Make(t, nil),
		Clock:            clock,
		DeploymentValues: deploymentValues,
	}}}
}

// newAISpendPeriodRequest returns a request with the given query parameters
// and a recorder for its response.
func newAISpendPeriodRequest(params map[string]string) (*http.Request, *httptest.ResponseRecorder) {
	query := url.Values{}
	for key, value := range params {
		query.Set(key, value)
	}
	return httptest.NewRequest(http.MethodGet, "/?"+query.Encode(), nil), httptest.NewRecorder()
}

// requireAISpendPeriodResult checks a period helper's result against the
// case: the period on success, or a 400 with the expected message.
func requireAISpendPeriodResult(t *testing.T, tc aiSpendPeriodCase, rw *httptest.ResponseRecorder, got aiSpendPeriod, ok bool) {
	t.Helper()
	if tc.wantMessage != "" {
		require.False(t, ok)
		require.Equal(t, http.StatusBadRequest, rw.Code)
		var resp codersdk.Response
		require.NoError(t, json.NewDecoder(rw.Body).Decode(&resp))
		require.Contains(t, resp.Message, tc.wantMessage)
		return
	}
	require.True(t, ok)
	require.Equal(t, tc.want, got)
}

// periodParams returns the query parameters for an explicit period.
func periodParams(start, end time.Time) map[string]string {
	return map[string]string{"period_start": start.Format(time.RFC3339), "period_end": end.Format(time.RFC3339)}
}

// TestAISpendPeriod tests the AI spend period for data that is not purged,
// so retention does not apply.
func TestAISpendPeriod(t *testing.T) {
	t.Parallel()

	week := 7 * 24 * time.Hour
	// March 2 is before a 7d retention window, which starts at noon on March 8.
	beforeRetention := aiSpendPeriodMonthStart.AddDate(0, 0, 1)

	cases := []aiSpendPeriodCase{
		{
			name: "DefaultPeriod",
			want: aiSpendPeriod{start: aiSpendPeriodMonthStart, end: aiSpendPeriodMonthEnd},
		},
		{
			name:   "ExplicitPeriod",
			params: periodParams(aiSpendPeriodMonthStart, aiSpendPeriodNow),
			want:   aiSpendPeriod{start: aiSpendPeriodMonthStart, end: aiSpendPeriodNow},
		},
		{
			// The daily spend aggregate is not purged, so retention does not
			// narrow or reject this period.
			name:      "DefaultPeriodIgnoresRetention",
			retention: week,
			want:      aiSpendPeriod{start: aiSpendPeriodMonthStart, end: aiSpendPeriodMonthEnd},
		},
		{
			name:      "ExplicitPeriodIgnoresRetention",
			retention: week,
			params:    periodParams(beforeRetention, beforeRetention.AddDate(0, 0, 1)),
			want:      aiSpendPeriod{start: beforeRetention, end: beforeRetention.AddDate(0, 0, 1)},
		},
		{
			name:        "OnlyStart",
			params:      map[string]string{"period_start": aiSpendPeriodMonthStart.Format(time.RFC3339)},
			wantMessage: "provided together",
		},
		{
			name:        "OnlyEnd",
			params:      map[string]string{"period_end": aiSpendPeriodMonthEnd.Format(time.RFC3339)},
			wantMessage: "provided together",
		},
		{
			name:        "StartNotBeforeEnd",
			params:      periodParams(aiSpendPeriodNow, aiSpendPeriodNow),
			wantMessage: "must be before",
		},
		{
			name:        "LongerThan31Days",
			params:      periodParams(aiSpendPeriodMonthStart, aiSpendPeriodMonthStart.AddDate(0, 0, 32)),
			wantMessage: "must not exceed 31 days",
		},
		{
			name:   "Exactly31Days",
			params: periodParams(aiSpendPeriodMonthStart, aiSpendPeriodMonthStart.AddDate(0, 0, 31)),
			want:   aiSpendPeriod{start: aiSpendPeriodMonthStart, end: aiSpendPeriodMonthStart.AddDate(0, 0, 31)},
		},
		{
			name:        "InvalidStart",
			params:      map[string]string{"period_start": "yesterday", "period_end": aiSpendPeriodMonthEnd.Format(time.RFC3339)},
			wantMessage: "have invalid values",
		},
		{
			name:        "UnknownParameter",
			params:      map[string]string{"limit": "10"},
			wantMessage: "have invalid values",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			api := newAISpendPeriodTestAPI(t, tc.retention)
			r, rw := newAISpendPeriodRequest(tc.params)

			got, ok := api.aiSpendPeriod(r.Context(), rw, r, httpapi.NewQueryParamParser())

			requireAISpendPeriodResult(t, tc, rw, got, ok)
		})
	}
}

// TestAISpendRetainedPeriod tests the AI spend period for data subject to AI
// Gateway data retention. Retention narrows the default period and rejects an
// explicit period that starts before the retention window.
func TestAISpendRetainedPeriod(t *testing.T) {
	t.Parallel()

	week := 7 * 24 * time.Hour
	weekRetentionStart := aiSpendPeriodNow.Add(-week)
	sixtyDays := 60 * 24 * time.Hour

	cases := []aiSpendPeriodCase{
		{
			name: "RetentionDisabled",
			want: aiSpendPeriod{start: aiSpendPeriodMonthStart, end: aiSpendPeriodMonthEnd},
		},
		{
			name:      "DefaultPeriodWithinRetention",
			retention: sixtyDays,
			want:      aiSpendPeriod{start: aiSpendPeriodMonthStart, end: aiSpendPeriodMonthEnd},
		},
		{
			name:      "DefaultPeriodNarrowedToRetention",
			retention: week,
			want:      aiSpendPeriod{start: weekRetentionStart, end: aiSpendPeriodMonthEnd},
		},
		{
			name:      "ExplicitPeriodWithinRetention",
			retention: week,
			params:    periodParams(weekRetentionStart, aiSpendPeriodNow),
			want:      aiSpendPeriod{start: weekRetentionStart, end: aiSpendPeriodNow},
		},
		{
			name:        "ExplicitPeriodBeforeRetention",
			retention:   week,
			params:      periodParams(weekRetentionStart.Add(-time.Hour), aiSpendPeriodNow),
			wantMessage: `Query parameter "period_start" is older than the configured AI Gateway data retention window (168h0m0s).`,
		},
		{
			name:   "RetentionDisabledAllowsOldPeriod",
			params: periodParams(aiSpendPeriodMonthStart.AddDate(-2, 0, 0), aiSpendPeriodMonthStart.AddDate(-2, 0, 1)),
			want:   aiSpendPeriod{start: aiSpendPeriodMonthStart.AddDate(-2, 0, 0), end: aiSpendPeriodMonthStart.AddDate(-2, 0, 1)},
		},
		{
			// Invalid input is rejected before retention applies.
			name:        "OnlyStart",
			retention:   week,
			params:      map[string]string{"period_start": aiSpendPeriodMonthStart.Format(time.RFC3339)},
			wantMessage: "provided together",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			api := newAISpendPeriodTestAPI(t, tc.retention)
			r, rw := newAISpendPeriodRequest(tc.params)

			got, ok := api.aiSpendRetainedPeriod(r.Context(), rw, r, httpapi.NewQueryParamParser())

			requireAISpendPeriodResult(t, tc, rw, got, ok)
		})
	}
}

func TestParseOrganizationAISpendPage(t *testing.T) {
	t.Parallel()

	limitRange := codersdk.ValidationError{Field: "limit", Detail: `Query param "limit" must be in range [1, 100].`}

	cases := []struct {
		name   string
		params map[string]string
		want   codersdk.OrganizationAISpendPage
		// wantErrors is every validation error, in order.
		wantErrors []codersdk.ValidationError
	}{
		{
			name:       "Default",
			want:       codersdk.OrganizationAISpendPage{Limit: 10},
			wantErrors: []codersdk.ValidationError{},
		},
		{
			name:       "LimitAndOffset",
			params:     map[string]string{"limit": "25", "offset": "50"},
			want:       codersdk.OrganizationAISpendPage{Limit: 25, Offset: 50},
			wantErrors: []codersdk.ValidationError{},
		},
		{
			name:       "LimitMax",
			params:     map[string]string{"limit": "100"},
			want:       codersdk.OrganizationAISpendPage{Limit: 100},
			wantErrors: []codersdk.ValidationError{},
		},
		{
			name:       "LimitAboveMax",
			params:     map[string]string{"limit": "101"},
			want:       codersdk.OrganizationAISpendPage{Limit: 101},
			wantErrors: []codersdk.ValidationError{limitRange},
		},
		{
			name:       "LimitZero",
			params:     map[string]string{"limit": "0"},
			want:       codersdk.OrganizationAISpendPage{},
			wantErrors: []codersdk.ValidationError{limitRange},
		},
		{
			name:   "NegativeOffset",
			params: map[string]string{"offset": "-1"},
			want:   codersdk.OrganizationAISpendPage{Limit: 10},
			wantErrors: []codersdk.ValidationError{
				{Field: "offset", Detail: `Query param "offset" must be a valid 32-bit positive integer: value is negative`},
			},
		},
		{
			// An unparsable limit is read as zero, so it also fails the range.
			name:   "InvalidLimit",
			params: map[string]string{"limit": "ten"},
			want:   codersdk.OrganizationAISpendPage{},
			wantErrors: []codersdk.ValidationError{
				{Field: "limit", Detail: `Query param "limit" must be a valid 32-bit positive integer: strconv.ParseInt: parsing "ten": invalid syntax`},
				limitRange,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			query := url.Values{}
			for key, value := range tc.params {
				query.Set(key, value)
			}
			parser := httpapi.NewQueryParamParser()

			got := parseOrganizationAISpendPage(parser, query)

			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantErrors, parser.Errors)
		})
	}
}

func TestParseAISpendUserIDs(t *testing.T) {
	t.Parallel()

	newUserIDs := func(count int) []uuid.UUID {
		userIDs := make([]uuid.UUID, 0, count)
		for range count {
			userIDs = append(userIDs, uuid.New())
		}
		return userIDs
	}
	joinUserIDs := func(userIDs []uuid.UUID) string {
		values := make([]string, 0, len(userIDs))
		for _, userID := range userIDs {
			values = append(values, userID.String())
		}
		return strings.Join(values, ",")
	}
	twoUserIDs := newUserIDs(2)
	maxUserIDs := newUserIDs(100)
	aboveMaxUserIDs := newUserIDs(101)

	cases := []struct {
		name   string
		params map[string]string
		want   []uuid.UUID
		// wantErrors is every validation error, in order.
		wantErrors []codersdk.ValidationError
	}{
		{
			name:       "Missing",
			want:       nil,
			wantErrors: []codersdk.ValidationError{},
		},
		{
			name:       "SeveralUserIDs",
			params:     map[string]string{"user_ids": joinUserIDs(twoUserIDs)},
			want:       twoUserIDs,
			wantErrors: []codersdk.ValidationError{},
		},
		{
			name:       "MaxUserIDs",
			params:     map[string]string{"user_ids": joinUserIDs(maxUserIDs)},
			want:       maxUserIDs,
			wantErrors: []codersdk.ValidationError{},
		},
		{
			name:   "AboveMaxUserIDs",
			params: map[string]string{"user_ids": joinUserIDs(aboveMaxUserIDs)},
			want:   aboveMaxUserIDs,
			wantErrors: []codersdk.ValidationError{
				{Field: "user_ids", Detail: "user_ids has 101 entries, maximum is 100."},
			},
		},
		{
			name:   "InvalidUserID",
			params: map[string]string{"user_ids": "not-a-uuid"},
			want:   []uuid.UUID{},
			wantErrors: []codersdk.ValidationError{
				{Field: "user_ids", Detail: `Query param "user_ids" has invalid values: invalid UUID length: 10`},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			query := url.Values{}
			for key, value := range tc.params {
				query.Set(key, value)
			}
			parser := httpapi.NewQueryParamParser()

			got := parseAISpendUserIDs(parser, query)

			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantErrors, parser.Errors)
		})
	}
}
