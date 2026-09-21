package coderd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/workspacedebug"
	"github.com/coder/coder/v2/codersdk"
)

// workspaceDebugReasoningEffort keeps debugging turns fast. A failed build is
// blocking the user, so a quick first answer beats a deep one; the model can
// still call tools for more detail.
const workspaceDebugReasoningEffort = "low"

// workspaceDebugSystemPrompt instructs the model for a debugging chat. The
// failure context bundle is appended after it.
const workspaceDebugSystemPrompt = `You are helping a Coder user whose workspace failed to build or start. The user is blocked, so be fast and concrete.

You do not have a running workspace. Do not attempt read_file, execute, or other workspace tools; they will fail. Use only the failure context below and the read-only tools get_workspace_build_logs, get_workspace_agent_logs, and get_template_version_files when the context is not enough.

Respond in this shape, in markdown, and keep it short:

1. **What failed**: one or two sentences naming the failing resource, step, or script.
2. **Evidence**: quote the one to three log lines that prove it.
3. **Fix**: the exact action to take. Distinguish clearly between actions the user can take themselves (retry, change a parameter, update the workspace to the active template version, fix their startup script or dotfiles, free quota) and changes that need a template administrator (Terraform, provider credentials, images, infrastructure limits).
4. **Who**: state whether the user can resolve this alone or needs an administrator, using the requesting user's roles and template access from the context. If an administrator is needed, write a two-sentence message the user can paste to them, including the workspace name, build number, and the key error line.

If the cause is genuinely unclear, say so, list the two most likely causes, and ask one focused question.`

// @Summary Get or create a workspace build debugging chat
// @ID get-or-create-workspace-build-debugging-chat
// @Security CoderSessionToken
// @Produce json
// @Tags Builds
// @Param workspacebuild path string true "Workspace build ID" format(uuid)
// @Success 200 {object} codersdk.WorkspaceDebugChatResponse
// @Success 201 {object} codersdk.WorkspaceDebugChatResponse
// @Router /api/experimental/workspacebuilds/{workspacebuild}/debug-chat [post]
func (api *API) postWorkspaceBuildDebugChat(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	build := httpmw.WorkspaceBuildParam(r)
	workspace := httpmw.WorkspaceParam(r)

	if !api.requireChatDaemon(ctx, rw) {
		return
	}

	// Members can open chats only for their own organization, matching the
	// rules of POST /chats. The workspace middleware already enforced read
	// access to the workspace.
	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceChat.WithOwner(apiKey.UserID.String()).InOrg(workspace.OrganizationID)) {
		httpapi.Forbidden(rw)
		return
	}

	labelFilter, err := json.Marshal(map[string]string{
		workspacedebug.LabelBuildID: build.ID.String(),
	})
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("marshal label filter: %w", err))
		return
	}
	existing, err := api.Database.GetChats(ctx, database.GetChatsParams{
		OwnedOnly:   true,
		ViewerID:    apiKey.UserID,
		LabelFilter: pqtype.NullRawMessage{RawMessage: labelFilter, Valid: true},
		LimitOpt:    1,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to look up existing debugging chat.",
			Detail:  err.Error(),
		})
		return
	}
	for _, row := range existing {
		if row.Chat.Archived {
			continue
		}
		chatFiles := api.fetchChatFileMetadata(ctx, row.Chat.ID)
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceDebugChatResponse{
			Chat:           db2sdk.Chat(row.Chat, nil, chatFiles),
			Created:        false,
			FailureSummary: row.Chat.Labels[workspacedebug.LabelFailure],
		})
		return
	}

	// Collect the bundle as the requesting user so RBAC decides which parts
	// of the deployment the model gets to see.
	bundle, err := workspacedebug.Collect(ctx, api.Database, build.ID)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to collect workspace failure context.",
			Detail:  err.Error(),
		})
		return
	}

	modelConfigID, _, status, modelErr := api.resolveCreateChatModelConfigID(ctx, apiKey.UserID, codersdk.CreateChatRequest{
		OrganizationID: workspace.OrganizationID,
	})
	if modelErr != nil {
		httpapi.Write(ctx, rw, status, *modelErr)
		return
	}

	aReq, commitAudit := audit.InitRequest[database.Chat](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: workspace.OrganizationID,
	})
	defer commitAudit()

	failure := bundle.FailureSummary()
	prompt := fmt.Sprintf(
		"This workspace failed to startup with %q error. Investigate why it failed and provide a resolving action the user can take, or if they need admin assistance.",
		failure,
	)
	labels := map[string]string{
		workspacedebug.LabelKind:        workspacedebug.LabelKindValue,
		workspacedebug.LabelWorkspaceID: workspace.ID.String(),
		workspacedebug.LabelBuildID:     build.ID.String(),
		workspacedebug.LabelFailure:     truncateLabelValue(failure),
	}
	effort := workspaceDebugReasoningEffort

	chat, err := api.chatDaemon.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     workspace.OrganizationID,
		OwnerID:            apiKey.UserID,
		Title:              fmt.Sprintf("Debug: %s build #%d", workspace.Name, build.BuildNumber),
		ModelConfigID:      modelConfigID,
		ReasoningEffort:    &effort,
		ClientType:         database.ChatClientTypeUi,
		SystemPrompt:       workspaceDebugSystemPrompt + "\n\n" + bundle.Prompt(),
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)},
		Labels:             labels,
	})
	if err != nil {
		if writeChatHookErr(ctx, rw, err, "Chat creation denied by lifecycle hook.") {
			return
		}
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create debugging chat.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = chat

	chat, err = api.Database.GetChatByID(ctx, chat.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to read back chat after creation.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = chat

	chatFiles := api.fetchChatFileMetadata(ctx, chat.ID)
	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.WorkspaceDebugChatResponse{
		Chat:           db2sdk.Chat(chat, nil, chatFiles),
		Created:        true,
		FailureSummary: failure,
	})
}

// truncateLabelValue keeps a failure summary within the chat label value
// limit.
func truncateLabelValue(value string) string {
	const maxLen = 250
	value = strings.TrimSpace(value)
	if len(value) <= maxLen {
		return value
	}
	return strings.ToValidUTF8(value[:maxLen], "")
}
