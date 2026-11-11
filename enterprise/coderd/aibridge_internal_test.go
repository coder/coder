package coderd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

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

type aiSpendPeriodFunc func(*API, http.ResponseWriter, *http.Request) (aiSpendPeriod, bool)

// runAISpendPeriodCases resolves each case's request with resolve, using a
// mock clock and the case's retention.
func runAISpendPeriodCases(t *testing.T, resolve aiSpendPeriodFunc, cases []aiSpendPeriodCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clock := quartz.NewMock(t)
			clock.Set(aiSpendPeriodNow)
			dv := &codersdk.DeploymentValues{}
			dv.AI.BridgeConfig.Retention = serpent.Duration(tc.retention)
			api := &API{Options: &Options{Options: &agplcoderd.Options{
				Logger:           slogtest.Make(t, nil),
				Clock:            clock,
				DeploymentValues: dv,
			}}}
			query := url.Values{}
			for k, v := range tc.params {
				query.Set(k, v)
			}
			r := httptest.NewRequest(http.MethodGet, "/?"+query.Encode(), nil)
			rw := httptest.NewRecorder()

			got, ok := resolve(api, rw, r)

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
		})
	}
}

// periodParams returns the query parameters for an explicit period.
func periodParams(start, end time.Time) map[string]string {
	return map[string]string{"period_start": start.Format(time.RFC3339), "period_end": end.Format(time.RFC3339)}
}

func TestAISpendPeriod(t *testing.T) {
	t.Parallel()

	week := 7 * 24 * time.Hour
	// March 2 is before a 7d retention window, which starts at noon on March 8.
	beforeRetention := aiSpendPeriodMonthStart.AddDate(0, 0, 1)

	runAISpendPeriodCases(t, func(api *API, rw http.ResponseWriter, r *http.Request) (aiSpendPeriod, bool) {
		return api.aiSpendPeriod(r.Context(), rw, r, httpapi.NewQueryParamParser())
	}, []aiSpendPeriodCase{
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
	})
}

func TestAISpendRetainedPeriod(t *testing.T) {
	t.Parallel()

	week := 7 * 24 * time.Hour
	weekRetentionStart := aiSpendPeriodNow.Add(-week)
	sixtyDays := 60 * 24 * time.Hour
	sixtyDayRetentionStart := aiSpendPeriodNow.Add(-sixtyDays)

	runAISpendPeriodCases(t, func(api *API, rw http.ResponseWriter, r *http.Request) (aiSpendPeriod, bool) {
		return api.aiSpendRetainedPeriod(r.Context(), rw, r, httpapi.NewQueryParamParser())
	}, []aiSpendPeriodCase{
		{
			name: "RetentionDisabled",
			want: aiSpendPeriod{start: aiSpendPeriodMonthStart, end: aiSpendPeriodMonthEnd},
		},
		{
			name:      "DefaultPeriodWithinRetention",
			retention: sixtyDays,
			want:      aiSpendPeriod{start: aiSpendPeriodMonthStart, end: aiSpendPeriodMonthEnd, retentionStart: sixtyDayRetentionStart},
		},
		{
			name:      "DefaultPeriodNarrowedToRetention",
			retention: week,
			want:      aiSpendPeriod{start: weekRetentionStart, end: aiSpendPeriodMonthEnd, retentionStart: weekRetentionStart},
		},
		{
			name:      "ExplicitPeriodWithinRetention",
			retention: week,
			params:    periodParams(weekRetentionStart, aiSpendPeriodNow),
			want:      aiSpendPeriod{start: weekRetentionStart, end: aiSpendPeriodNow, retentionStart: weekRetentionStart},
		},
		{
			name:        "ExplicitPeriodBeforeRetention",
			retention:   week,
			params:      periodParams(weekRetentionStart.Add(-time.Hour), aiSpendPeriodNow),
			wantMessage: `Query parameter "period_start" must not be before 2026-03-08T12:00:00Z, the start of the configured AI Gateway data retention window.`,
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
	})
}
