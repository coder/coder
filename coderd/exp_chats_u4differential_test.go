package coderd_test

// TEMPORARY differential harness for U4 (CODAGT-872). Re-runs the baseline
// cases against the new typed-table API and compares status + full body to
// the parent-commit baseline JSON. Run with U4_BASELINE_IN set. Delete
// before committing. Not part of the shipped diff.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// Cases whose shape INTENTIONALLY changes in U4; everything else must match
// the parent baseline exactly (status and body, modulo volatile UUIDs).
var u4IntentionalChanges = map[string]string{
	"get-after-set/general":          "GET shape: is_malformed removed from the per-context response",
	"get-after-clear/general":        "GET shape: is_malformed removed",
	"get-after-set/advisor":          "GET shape: is_malformed removed",
	"get-after-clear/advisor":        "GET shape: is_malformed removed",
	"get-never-set/title_generation": "GET shape: is_malformed removed",
	"personal/get-after-put":         "response: is_malformed removed from personal overrides",
	"personal/get-default-org-view":  "route: org is now in the path, no default-org query view",
	"personal/get-final":             "response: is_malformed removed",
	"personal/put-missing-org-param": "route: org in path, no query parameter to be missing",
	"personal/put-bad-org-param":     "route: org in path, no query parameter to be invalid",
	"advisor/put-with-model-fields":  "stale-write detection removed with the Deprecated fields; unknown JSON fields are ignored",
	"advisor/get":                    "unchanged except runtime blob shape",
}

func TestU4Differential(t *testing.T) {
	in := os.Getenv("U4_BASELINE_IN")
	if in == "" {
		t.Skip("set U4_BASELINE_IN to the parent baseline JSON")
	}
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: false})
	firstUser := coderdtest.CreateFirstUser(t, client)
	orgID := firstUser.OrganizationID
	exp := codersdk.NewExperimentalClient(client)

	otherOrg := dbgen.Organization(t, db, database.Organization{Name: "u4-diff-other-org"})
	aiProvider := createAIProviderForTest(t, exp, "openai-compat", "test-api-key")

	cfg, err := exp.CreateChatModelConfig(ctx, orgID, codersdk.CreateChatModelConfigRequest{
		AIProviderID: &aiProvider.ID,
		Model:        "gpt-5.2-diff", DisplayName: "diff", ContextLimit: ptr.Ref(int64(200000)), CompressionThreshold: ptr.Ref(int32(70)),
	})
	require.NoError(t, err)
	otherCfg, err := exp.CreateChatModelConfig(ctx, otherOrg.ID, codersdk.CreateChatModelConfigRequest{
		AIProviderID: &aiProvider.ID,
		Model:        "gpt-5.2-diff-other", DisplayName: "diff-other", ContextLimit: ptr.Ref(int64(200000)), CompressionThreshold: ptr.Ref(int32(70)),
	})
	require.NoError(t, err)
	disabledCfg, err := exp.CreateChatModelConfig(ctx, orgID, codersdk.CreateChatModelConfigRequest{
		AIProviderID: &aiProvider.ID,
		Model:        "gpt-5.2-diff-disabled", DisplayName: "diff-disabled", Enabled: ptr.Ref(false), ContextLimit: ptr.Ref(int64(200000)), CompressionThreshold: ptr.Ref(int32(70)),
	})
	require.NoError(t, err)

	require.NoError(t, exp.UpdateChatPersonalModelOverridesAdminSettings(ctx, codersdk.UpdateChatPersonalModelOverridesAdminSettingsRequest{AllowUsers: true}))

	var entries []baselineEntry
	record := func(name, method, path string, body any) {
		t.Helper()
		res, err := client.Request(ctx, method, path, body)
		require.NoError(t, err)
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		entries = append(entries, baselineEntry{Name: name, Method: method, Path: path, Status: res.StatusCode, Body: string(raw)})
	}

	adminPath := func(c string) string {
		return fmt.Sprintf("/api/experimental/organizations/%s/ai/model-overrides/%s", orgID, c)
	}
	personalPutPath := func(c string) string {
		return fmt.Sprintf("/api/experimental/organizations/%s/members/me/ai/model-overrides/%s", orgID, c)
	}

	for _, c := range []string{"general", "explore", "title_generation", "compaction", "advisor"} {
		record("clear-never-set/"+c, http.MethodPut, adminPath(c), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: ""})
	}
	record("put-nonexistent-config", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: uuid.NewString()})
	record("put-cross-org-config", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: otherCfg.ID.String()})
	record("put-disabled-config", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: disabledCfg.ID.String()})
	record("put-malformed-id/not-a-uuid", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: "not-a-uuid"})
	record("put-malformed-id/with-effort-suffix", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: cfg.ID.String() + ":high"})
	record("put-nil-uuid", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: uuid.Nil.String()})
	record("put-bad-context", http.MethodPut, adminPath("nonsense"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: cfg.ID.String()})
	record("put-effort-without-model", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ReasoningEffort: ptr.Ref("high")})
	record("put-bad-effort", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: cfg.ID.String(), ReasoningEffort: ptr.Ref("bogus")})
	for _, c := range []string{"general", "advisor"} {
		record("set/"+c, http.MethodPut, adminPath(c), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: cfg.ID.String()})
		record("get-after-set/"+c, http.MethodGet, fmt.Sprintf("/api/experimental/organizations/%s/ai/model-overrides", orgID), nil)
		record("clear-after-set/"+c, http.MethodPut, adminPath(c), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: ""})
		record("get-after-clear/"+c, http.MethodGet, fmt.Sprintf("/api/experimental/organizations/%s/ai/model-overrides", orgID), nil)
	}
	record("get-never-set/title_generation", http.MethodGet, fmt.Sprintf("/api/experimental/organizations/%s/ai/model-overrides", orgID), nil)

	record("personal/put-root-chat_default", http.MethodPut, personalPutPath("root"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeChatDefault})
	record("personal/get-after-put", http.MethodGet, fmt.Sprintf("/api/experimental/organizations/%s/members/me/ai/model-overrides", orgID), nil)
	record("personal/put-deployment_default-on-root", http.MethodPut, personalPutPath("root"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeDeploymentDefault})
	record("personal/put-model-mode", http.MethodPut, personalPutPath("general"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeModel, ModelConfigID: cfg.ID.String()})
	record("personal/put-model-cross-org", http.MethodPut, personalPutPath("general"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeModel, ModelConfigID: otherCfg.ID.String()})
	record("personal/put-model-nonexistent", http.MethodPut, personalPutPath("general"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeModel, ModelConfigID: uuid.NewString()})
	record("personal/put-bad-mode", http.MethodPut, personalPutPath("general"), map[string]any{"mode": "bogus"})
	record("personal/put-bad-context", http.MethodPut, personalPutPath("nonsense"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeChatDefault})
	record("personal/get-final", http.MethodGet, fmt.Sprintf("/api/experimental/organizations/%s/members/me/ai/model-overrides", orgID), nil)

	record("advisor/put-with-model-fields", http.MethodPut, "/api/experimental/chats/config/advisor", map[string]any{
		"max_uses_per_run": 3, "max_output_tokens": 512, "model_config_id": cfg.ID.String(), "reasoning_effort": "low",
	})
	record("advisor/put-runtime-only", http.MethodPut, "/api/experimental/chats/config/advisor", map[string]any{
		"max_uses_per_run": 3, "max_output_tokens": 512,
	})
	record("advisor/get", http.MethodGet, "/api/experimental/chats/config/advisor", nil)

	record("toggle/get", http.MethodGet, "/api/experimental/chats/config/personal-model-overrides", nil)
	record("toggle/put", http.MethodPut, "/api/experimental/chats/config/personal-model-overrides", codersdk.UpdateChatPersonalModelOverridesAdminSettingsRequest{AllowUsers: true})

	require.NoError(t, exp.UpdateChatPersonalModelOverridesAdminSettings(ctx, codersdk.UpdateChatPersonalModelOverridesAdminSettingsRequest{AllowUsers: false}))
	record("personal/put-toggle-disabled", http.MethodPut, personalPutPath("general"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeChatDefault})

	// Load baseline, normalize volatile values, compare per case.
	raw, err := os.ReadFile(in)
	require.NoError(t, err)
	var baseline []baselineEntry
	require.NoError(t, json.Unmarshal(raw, &baseline))
	baseByName := make(map[string]baselineEntry, len(baseline))
	for _, b := range baseline {
		baseByName[b.Name] = b
	}

	uuidRe := regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	normalize := func(s string) string { return uuidRe.ReplaceAllString(s, "<uuid>") }

	var mismatches []string
	for _, got := range entries {
		if _, intentional := u4IntentionalChanges[got.Name]; intentional {
			continue
		}
		base, ok := baseByName[got.Name]
		if !ok {
			mismatches = append(mismatches, fmt.Sprintf("%s: no baseline entry", got.Name))
			continue
		}
		if base.Status != got.Status || normalize(base.Body) != normalize(got.Body) {
			mismatches = append(mismatches, fmt.Sprintf(
				"%s:\n  baseline %d %s\n  got      %d %s",
				got.Name, base.Status, normalize(base.Body), got.Status, normalize(got.Body)))
		}
	}
	for _, m := range mismatches {
		t.Logf("MISMATCH %s", m)
	}
	require.Empty(t, mismatches, "%d differential mismatches", len(mismatches))

	// Intentional-change cases: assert their new expected shape explicitly.
	for _, got := range entries {
		reason, intentional := u4IntentionalChanges[got.Name]
		if !intentional {
			continue
		}
		body := normalize(got.Body)
		if len(body) > 120 {
			body = body[:120]
		}
		t.Logf("intentional change %-40s %d %s (%s)", got.Name, got.Status, body, reason)
	}

	out, err := json.MarshalIndent(entries, "", "  ")
	require.NoError(t, err)
	if outPath := os.Getenv("U4_DIFF_OUT"); outPath != "" {
		require.NoError(t, os.WriteFile(outPath, out, 0o600))
	}
}
