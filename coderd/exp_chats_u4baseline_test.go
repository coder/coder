package coderd_test

// TEMPORARY baseline harness for U4 (CODAGT-872). Records status + full body
// for every override surface whose shape changes in this unit. Run against
// the parent commit, save JSON, delete this file before committing. Not part
// of the shipped diff.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

type baselineEntry struct {
	Name   string `json:"name"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
	Body   string `json:"body"`
}

func TestU4BaselineCapture(t *testing.T) {
	if os.Getenv("U4_BASELINE_OUT") == "" {
		t.Skip("set U4_BASELINE_OUT to record")
	}
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: false})
	firstUser := coderdtest.CreateFirstUser(t, client)
	orgID := firstUser.OrganizationID
	exp := codersdk.NewExperimentalClient(client)

	// Second org + config in it (cross-org cases). AGPL cannot create orgs
	// over HTTP (405), so seed directly.
	otherOrg := dbgen.Organization(t, db, database.Organization{Name: "u4-baseline-other-org"})
	aiProvider := createAIProviderForTest(t, exp, "openai-compat", "test-api-key")

	cfg, err := exp.CreateChatModelConfig(ctx, orgID, codersdk.CreateChatModelConfigRequest{
		AIProviderID: &aiProvider.ID,
		Model:        "gpt-5.2-baseline", DisplayName: "baseline", ContextLimit: ptr.Ref(int64(200000)), CompressionThreshold: ptr.Ref(int32(70)),
	})
	require.NoError(t, err)
	otherCfg, err := exp.CreateChatModelConfig(ctx, otherOrg.ID, codersdk.CreateChatModelConfigRequest{
		AIProviderID: &aiProvider.ID,
		Model:        "gpt-5.2-baseline-other", DisplayName: "baseline-other", ContextLimit: ptr.Ref(int64(200000)), CompressionThreshold: ptr.Ref(int32(70)),
	})
	require.NoError(t, err)
	disabledCfg, err := exp.CreateChatModelConfig(ctx, orgID, codersdk.CreateChatModelConfigRequest{
		AIProviderID: &aiProvider.ID,
		Model:        "gpt-5.2-baseline-disabled", DisplayName: "baseline-disabled", Enabled: ptr.Ref(false), ContextLimit: ptr.Ref(int64(200000)), CompressionThreshold: ptr.Ref(int32(70)),
	})
	require.NoError(t, err)

	// Enable personal overrides via the toggle (must stay).
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
		return fmt.Sprintf("/api/experimental/organizations/%s/chats/config/model-override/%s", orgID, c)
	}
	personalPutPath := func(c string) string {
		return fmt.Sprintf("/api/experimental/chats/config/user-personal-model-overrides/%s?organization=%s", c, orgID)
	}

	// 1. Clear-of-never-set (admin), every context.
	for _, c := range []string{"general", "explore", "title_generation", "compaction", "advisor"} {
		record("clear-never-set/"+c, http.MethodPut, adminPath(c), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: ""})
	}
	// 2. Override naming a nonexistent config.
	record("put-nonexistent-config", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: uuid.NewString()})
	// 3. Override naming another org's config.
	record("put-cross-org-config", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: otherCfg.ID.String()})
	// 4. Disabled config.
	record("put-disabled-config", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: disabledCfg.ID.String()})
	// 5. Malformed model_config_id shapes the old parser rejected.
	record("put-malformed-id/not-a-uuid", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: "not-a-uuid"})
	record("put-malformed-id/with-effort-suffix", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: cfg.ID.String() + ":high"})
	record("put-nil-uuid", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: uuid.Nil.String()})
	// 6. Bad context.
	record("put-bad-context", http.MethodPut, adminPath("nonsense"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: cfg.ID.String()})
	// 7. Reasoning effort without model, bad effort value.
	record("put-effort-without-model", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ReasoningEffort: ptr.Ref("high")})
	record("put-bad-effort", http.MethodPut, adminPath("general"), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: cfg.ID.String(), ReasoningEffort: ptr.Ref("bogus")})
	// 8. Happy path set then GET per context, then clear (delete semantics baseline: today clear = empty string).
	for _, c := range []string{"general", "advisor"} {
		record("set/"+c, http.MethodPut, adminPath(c), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: cfg.ID.String()})
		record("get-after-set/"+c, http.MethodGet, adminPath(c), nil)
		record("clear-after-set/"+c, http.MethodPut, adminPath(c), codersdk.UpdateChatModelOverrideRequest{ModelConfigID: ""})
		record("get-after-clear/"+c, http.MethodGet, adminPath(c), nil)
	}
	// 9. GET of never-set context.
	record("get-never-set/title_generation", http.MethodGet, adminPath("title_generation"), nil)

	// 10. Personal: PUT with required ?organization= (ruling 2 target).
	record("personal/put-root-chat_default", http.MethodPut, personalPutPath("root"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeChatDefault})
	record("personal/get-after-put", http.MethodGet, "/api/experimental/chats/config/user-personal-model-overrides?organization="+orgID.String(), nil)
	record("personal/get-default-org-view", http.MethodGet, "/api/experimental/chats/config/user-personal-model-overrides", nil)
	record("personal/put-missing-org-param", http.MethodPut, "/api/experimental/chats/config/user-personal-model-overrides/root", codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeChatDefault})
	record("personal/put-bad-org-param", http.MethodPut, "/api/experimental/chats/config/user-personal-model-overrides/root?organization=not-a-uuid", codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeChatDefault})
	record("personal/put-deployment_default-on-root", http.MethodPut, personalPutPath("root"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeDeploymentDefault})
	record("personal/put-model-mode", http.MethodPut, personalPutPath("general"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeModel, ModelConfigID: cfg.ID.String()})
	record("personal/put-model-cross-org", http.MethodPut, personalPutPath("general"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeModel, ModelConfigID: otherCfg.ID.String()})
	record("personal/put-model-nonexistent", http.MethodPut, personalPutPath("general"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeModel, ModelConfigID: uuid.NewString()})
	record("personal/put-bad-mode", http.MethodPut, personalPutPath("general"), map[string]any{"mode": "bogus"})
	record("personal/put-bad-context", http.MethodPut, personalPutPath("nonsense"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeChatDefault})
	record("personal/get-final", http.MethodGet, "/api/experimental/chats/config/user-personal-model-overrides?organization="+orgID.String(), nil)

	// 11. Advisor runtime config PUT with deprecated model fields (stale client detection).
	record("advisor/put-with-model-fields", http.MethodPut, "/api/experimental/chats/config/advisor", map[string]any{
		"max_uses_per_run": 3, "max_output_tokens": 512, "model_config_id": cfg.ID.String(), "reasoning_effort": "low",
	})
	record("advisor/put-runtime-only", http.MethodPut, "/api/experimental/chats/config/advisor", map[string]any{
		"max_uses_per_run": 3, "max_output_tokens": 512,
	})
	record("advisor/get", http.MethodGet, "/api/experimental/chats/config/advisor", nil)

	// 12. Toggle routes (must stay byte-identical behavior).
	record("toggle/get", http.MethodGet, "/api/experimental/chats/config/personal-model-overrides", nil)
	record("toggle/put", http.MethodPut, "/api/experimental/chats/config/personal-model-overrides", codersdk.UpdateChatPersonalModelOverridesAdminSettingsRequest{AllowUsers: true})

	// 13. Personal write with toggle disabled.
	require.NoError(t, exp.UpdateChatPersonalModelOverridesAdminSettings(ctx, codersdk.UpdateChatPersonalModelOverridesAdminSettingsRequest{AllowUsers: false}))
	record("personal/put-toggle-disabled", http.MethodPut, personalPutPath("general"), codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeChatDefault})

	out, err := json.MarshalIndent(entries, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(os.Getenv("U4_BASELINE_OUT"), out, 0o600))
	t.Logf("recorded %d entries to %s", len(entries), os.Getenv("U4_BASELINE_OUT"))
}
