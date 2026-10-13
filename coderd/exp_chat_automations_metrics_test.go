package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// TestChatAutomationWebhookRejectedMetric checks that the webhook endpoint
// counts requests it rejects before they become a run, by reason only, and
// counts runs by trigger, outcome, and reason.
func TestChatAutomationWebhookRejectedMetric(t *testing.T) {
	t.Parallel()
	env := newChatAutomationTestEnv(t, nil, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	created, err := env.member.CreateChatAutomation(ctx, env.orgID, env.webhookRequest())
	require.NoError(t, err)
	registry := env.api.PrometheusRegistry

	status, _ := postChatAutomationEvent(t, env.member, created.Automation.ID, created.WebhookSecret+"x", []byte(`{}`))
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = postChatAutomationEvent(t, env.member, created.Automation.ID, "", []byte(`{}`))
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = postChatAutomationEvent(t, env.member, uuid.New(), created.WebhookSecret, []byte(`{}`))
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = postChatAutomationEvent(t, env.member, created.Automation.ID, created.WebhookSecret, []byte(`{}`))
	require.Equal(t, http.StatusAccepted, status)

	count := func(name string, labels map[string]string) float64 {
		t.Helper()
		families, err := registry.Gather()
		require.NoError(t, err)
		for _, family := range families {
			if family.GetName() != name {
				continue
			}
			for _, metric := range family.GetMetric() {
				matches := true
				got := map[string]string{}
				for _, label := range metric.GetLabel() {
					got[label.GetName()] = label.GetValue()
				}
				for k, v := range labels {
					if got[k] != v {
						matches = false
					}
				}
				if matches && len(got) == len(labels) {
					return metric.GetCounter().GetValue()
				}
			}
		}
		return 0
	}
	require.Equal(t, 2.0, count("coderd_agents_automation_webhook_rejected_total", map[string]string{"reason": "invalid_secret"}))
	require.Equal(t, 1.0, count("coderd_agents_automation_webhook_rejected_total", map[string]string{"reason": "unknown_automation"}))
	require.Equal(t, 1.0, count("coderd_agents_automation_runs_total", map[string]string{"trigger": "webhook", "outcome": "accepted", "reason": "none"}))
}
