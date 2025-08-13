package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// This endpoint is experimental and not guaranteed to be stable, so we're not
// generating public-facing documentation for it.
func (api *API) aiTasksPrompts(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	buildIDsParam := r.URL.Query().Get("build_ids")
	if buildIDsParam == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "build_ids query parameter is required",
		})
		return
	}

	// Parse build IDs
	buildIDStrings := strings.Split(buildIDsParam, ",")
	buildIDs := make([]uuid.UUID, 0, len(buildIDStrings))
	for _, idStr := range buildIDStrings {
		id, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Invalid build ID format: %s", idStr),
				Detail:  err.Error(),
			})
			return
		}
		buildIDs = append(buildIDs, id)
	}

	parameters, err := api.Database.GetWorkspaceBuildParametersByBuildIDs(ctx, buildIDs)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build parameters.",
			Detail:  err.Error(),
		})
		return
	}

	promptsByBuildID := make(map[string]string, len(parameters))
	for _, param := range parameters {
		if param.Name != codersdk.AITaskPromptParameterName {
			continue
		}
		buildID := param.WorkspaceBuildID.String()
		promptsByBuildID[buildID] = param.Value
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AITasksPromptsResponse{
		Prompts: promptsByBuildID,
	})
}

func (api *API) generateTaskName(ctx context.Context, prompt, fallback string) (string, error) {
	var (
		stream aisdk.DataStream
		err    error
	)

	conversation := []aisdk.Message{
		{
			Role: "system",
			Parts: []aisdk.Part{{
				Type: aisdk.PartTypeText,
				Text: `
					You are a task summarizer.
					You summarize AI prompts into workspace names.
					You will only respond with a workspace name.
					The workspace name **MUST** follow this regex /^[a-z0-9]+(?:-[a-z0-9]+)*$/
					The workspace name **MUST** be 32 characters or **LESS**.
					The workspace name **MUST** be all lower case.
					The workspace name **MUST** end in a number between 0 and 100.
					The workspace name **MUST** be prefixed with "task".
				`,
			}},
		},
		{
			Role: "user",
			Parts: []aisdk.Part{{
				Type: aisdk.PartTypeText,
				Text: prompt,
			}},
		},
	}

	if anthropicClient := api.anthropicClient.Load(); anthropicClient != nil {
		stream, err = anthropicDataStream(ctx, *anthropicClient, conversation)
		if err != nil {
			return "", xerrors.Errorf("create anthropic data stream: %w", err)
		}
	} else {
		return fallback, nil
	}

	var acc aisdk.DataStreamAccumulator
	stream = stream.WithAccumulator(&acc)

	if err := stream.Pipe(io.Discard); err != nil {
		return "", err
	}

	if len(acc.Messages()) == 0 {
		return fallback, nil
	}

	return acc.Messages()[0].Content, nil
}

func anthropicDataStream(ctx context.Context, client anthropic.Client, input []aisdk.Message) (aisdk.DataStream, error) {
	messages, system, err := aisdk.MessagesToAnthropic(input)
	if err != nil {
		return nil, xerrors.Errorf("convert messages to anthropic format: %w", err)
	}

	return aisdk.AnthropicToDataStream(client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude3_5HaikuLatest,
		MaxTokens: 24,
		System:    system,
		Messages:  messages,
	})), nil
}

// This endpoint is experimental and not guaranteed to be stable, so we're not
// generating public-facing documentation for it.
func (api *API) tasksCreate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		apiKey  = httpmw.APIKey(r)
		auditor = api.Auditor.Load()
		mems    = httpmw.OrganizationMembersParam(r)
	)

	var req codersdk.CreateTaskRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	hasAITask, err := api.Database.GetTemplateVersionHasAITask(ctx, req.TemplateVersionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || rbac.IsUnauthorizedError(err) {
			httpapi.ResourceNotFound(rw)
			return
		}

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching whether the template version has an AI task.",
			Detail:  err.Error(),
		})
		return
	}
	if !hasAITask {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf(`Template does not have required parameter %q`, codersdk.AITaskPromptParameterName),
		})
		return
	}

	taskName, err := api.generateTaskName(ctx, req.Prompt, req.Name)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error generating name for task.",
			Detail:  err.Error(),
		})
		return
	}

	if taskName == "" {
		taskName = req.Name
	}

	createReq := codersdk.CreateWorkspaceRequest{
		Name:                    taskName,
		TemplateVersionID:       req.TemplateVersionID,
		TemplateVersionPresetID: req.TemplateVersionPresetID,
		RichParameterValues: []codersdk.WorkspaceBuildParameter{
			{Name: codersdk.AITaskPromptParameterName, Value: req.Prompt},
		},
	}

	var owner workspaceOwner
	if mems.User != nil {
		// This user fetch is an optimization path for the most common case of creating a
		// task for 'Me'.
		//
		// This is also required to allow `owners` to create workspaces for users
		// that are not in an organization.
		owner = workspaceOwner{
			ID:        mems.User.ID,
			Username:  mems.User.Username,
			AvatarURL: mems.User.AvatarURL,
		}
	} else {
		// A task can still be created if the caller can read the organization
		// member. The organization is required, which can be sourced from the
		// template.
		//
		// TODO: This code gets called twice for each workspace build request.
		//   This is inefficient and costs at most 2 extra RTTs to the DB.
		//   This can be optimized. It exists as it is now for code simplicity.
		//   The most common case is to create a workspace for 'Me'. Which does
		//   not enter this code branch.
		template, ok := requestTemplate(ctx, rw, createReq, api.Database)
		if !ok {
			return
		}

		// If the caller can find the organization membership in the same org
		// as the template, then they can continue.
		orgIndex := slices.IndexFunc(mems.Memberships, func(mem httpmw.OrganizationMember) bool {
			return mem.OrganizationID == template.OrganizationID
		})
		if orgIndex == -1 {
			httpapi.ResourceNotFound(rw)
			return
		}

		member := mems.Memberships[orgIndex]
		owner = workspaceOwner{
			ID:        member.UserID,
			Username:  member.Username,
			AvatarURL: member.AvatarURL,
		}
	}

	aReq, commitAudit := audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionCreate,
		AdditionalFields: audit.AdditionalFields{
			WorkspaceOwner: owner.Username,
		},
	})

	defer commitAudit()
	createWorkspace(ctx, aReq, apiKey.UserID, api, owner, createReq, rw, r)
}
