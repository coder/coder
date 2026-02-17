package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"go.jetify.com/ai"
	aiapi "go.jetify.com/ai/api"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/util/namesgenerator"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

var (
	promptTokenPattern           = regexp.MustCompile(`[a-z0-9]+`)
	templateSelectionUUIDPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
)

const (
	chatWorkspaceBuildLogStreamTimeout = 2 * time.Minute
	chatWorkspaceBuildLogPollInterval  = 2 * time.Second
)

// WorkspaceCreatorAdapter provides coderd-specific operations for chat
// workspace creation.
type WorkspaceCreatorAdapter interface {
	PrepareWorkspaceCreate(
		ctx context.Context,
		chat database.Chat,
	) (context.Context, *http.Request, string, error)
	AuthorizedTemplates(ctx context.Context, r *http.Request) ([]database.Template, error)
	CreateWorkspace(
		ctx context.Context,
		r *http.Request,
		ownerID uuid.UUID,
		req codersdk.CreateWorkspaceRequest,
	) (codersdk.Workspace, error)
	Database() database.Store
	Pubsub() pubsub.Pubsub
	Logger() slog.Logger
}

// WorkspaceCreatorAdapterFuncs adapts simple callbacks to WorkspaceCreatorAdapter.
type WorkspaceCreatorAdapterFuncs struct {
	PrepareWorkspaceCreateFunc func(
		ctx context.Context,
		chat database.Chat,
	) (context.Context, *http.Request, string, error)
	AuthorizedTemplatesFunc func(
		ctx context.Context,
		r *http.Request,
	) ([]database.Template, error)
	CreateWorkspaceFunc func(
		ctx context.Context,
		r *http.Request,
		ownerID uuid.UUID,
		req codersdk.CreateWorkspaceRequest,
	) (codersdk.Workspace, error)
	DatabaseStore database.Store
	PubsubStore   pubsub.Pubsub
	LoggerStore   slog.Logger
}

func (a WorkspaceCreatorAdapterFuncs) PrepareWorkspaceCreate(
	ctx context.Context,
	chat database.Chat,
) (context.Context, *http.Request, string, error) {
	if a.PrepareWorkspaceCreateFunc == nil {
		return nil, nil, "", xerrors.New("chat workspace creator prepare callback is not configured")
	}
	return a.PrepareWorkspaceCreateFunc(ctx, chat)
}

func (a WorkspaceCreatorAdapterFuncs) AuthorizedTemplates(
	ctx context.Context,
	r *http.Request,
) ([]database.Template, error) {
	if a.AuthorizedTemplatesFunc == nil {
		return nil, xerrors.New("chat workspace creator templates callback is not configured")
	}
	return a.AuthorizedTemplatesFunc(ctx, r)
}

func (a WorkspaceCreatorAdapterFuncs) CreateWorkspace(
	ctx context.Context,
	r *http.Request,
	ownerID uuid.UUID,
	req codersdk.CreateWorkspaceRequest,
) (codersdk.Workspace, error) {
	if a.CreateWorkspaceFunc == nil {
		return codersdk.Workspace{}, xerrors.New("chat workspace creator create callback is not configured")
	}
	return a.CreateWorkspaceFunc(ctx, r, ownerID, req)
}

func (a WorkspaceCreatorAdapterFuncs) Database() database.Store {
	return a.DatabaseStore
}

func (a WorkspaceCreatorAdapterFuncs) Pubsub() pubsub.Pubsub {
	return a.PubsubStore
}

func (a WorkspaceCreatorAdapterFuncs) Logger() slog.Logger {
	return a.LoggerStore
}

type workspaceCreator struct {
	adapter WorkspaceCreatorAdapter
}

// NewWorkspaceCreator returns the default create-workspace implementation used
// by the chat processor.
func NewWorkspaceCreator(adapter WorkspaceCreatorAdapter) WorkspaceCreator {
	return &workspaceCreator{adapter: adapter}
}

type templateSelectionReasonError struct {
	reason string
}

func (e *templateSelectionReasonError) Error() string {
	return e.reason
}

func newTemplateSelectionReasonError(reason string) error {
	return &templateSelectionReasonError{reason: strings.TrimSpace(reason)}
}

func templateSelectionReason(err error) (string, bool) {
	var reasonErr *templateSelectionReasonError
	if !errors.As(err, &reasonErr) || reasonErr == nil || reasonErr.reason == "" {
		return "", false
	}
	return reasonErr.reason, true
}

func (c *workspaceCreator) CreateWorkspace(
	ctx context.Context,
	req CreateWorkspaceToolRequest,
) (CreateWorkspaceToolResult, error) {
	if c == nil || c.adapter == nil {
		return CreateWorkspaceToolResult{}, xerrors.New("chat workspace creator is not configured")
	}
	if c.adapter.Database() == nil {
		return CreateWorkspaceToolResult{}, xerrors.New("chat workspace creator database is not configured")
	}

	ctx, httpReq, accessURL, err := c.adapter.PrepareWorkspaceCreate(ctx, req.Chat)
	if err != nil {
		return CreateWorkspaceToolResult{}, err
	}
	accessURL = strings.TrimRight(accessURL, "/")

	spec, err := ParseWorkspaceSpec(req.Spec)
	if err != nil {
		return CreateWorkspaceToolResult{
			Created: false,
			Reason:  fmt.Sprintf("invalid workspace request format: %s", err),
		}, nil
	}

	createReq := spec.CreateRequest
	if strings.TrimSpace(createReq.Name) == "" && strings.TrimSpace(spec.Name) != "" {
		createReq.Name = strings.TrimSpace(spec.Name)
	}

	template, err := c.resolveTemplateSelection(ctx, httpReq, req, spec, &createReq)
	if err != nil {
		if reason, ok := templateSelectionReason(err); ok {
			return CreateWorkspaceToolResult{
				Created: false,
				Reason:  reason,
			}, nil
		}
		return CreateWorkspaceToolResult{}, err
	}

	if err := FinalizeWorkspaceName(&createReq, spec, req.Prompt, template.Name); err != nil {
		return CreateWorkspaceToolResult{
			Created: false,
			Reason:  err.Error(),
		}, nil
	}

	workspace, err := c.adapter.CreateWorkspace(ctx, httpReq, req.Chat.OwnerID, createReq)
	if err != nil {
		return CreateWorkspaceToolResult{}, err
	}

	if req.BuildLogHandler != nil {
		c.streamWorkspaceBuildLogs(ctx, workspace.LatestBuild.Job.ID, req.BuildLogHandler)
	}

	workspaceAgentID := uuid.Nil
	agents, err := c.adapter.Database().GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
	if err == nil && len(agents) > 0 {
		workspaceAgentID = agents[0].ID
	}

	workspaceURL := ""
	if accessURL != "" {
		workspaceURL = fmt.Sprintf("%s/@%s/%s", accessURL, workspace.OwnerName, workspace.Name)
	}

	return CreateWorkspaceToolResult{
		Created:          true,
		WorkspaceID:      workspace.ID,
		WorkspaceAgentID: workspaceAgentID,
		WorkspaceName:    workspace.FullName(),
		WorkspaceURL:     workspaceURL,
	}, nil
}

// WorkspaceSpec is a normalized create-workspace tool payload.
type WorkspaceSpec struct {
	CreateRequest codersdk.CreateWorkspaceRequest

	Name         string
	NameExplicit bool

	TemplateID       uuid.UUID
	TemplateRef      string
	TemplateExplicit bool

	TemplateVersionID       uuid.UUID
	TemplateVersionRef      string
	TemplateVersionExplicit bool
}

// ParseWorkspaceSpec decodes a create-workspace tool payload into a normalized
// shape that supports common alias keys.
func ParseWorkspaceSpec(raw json.RawMessage) (WorkspaceSpec, error) {
	spec := WorkspaceSpec{}
	if len(raw) == 0 || string(raw) == "null" {
		return spec, nil
	}

	if err := json.Unmarshal(raw, &spec.CreateRequest); err != nil {
		return WorkspaceSpec{}, xerrors.Errorf("decode create workspace request: %w", err)
	}

	if spec.CreateRequest.TemplateID != uuid.Nil {
		spec.TemplateID = spec.CreateRequest.TemplateID
		spec.TemplateExplicit = true
	}
	if spec.CreateRequest.TemplateVersionID != uuid.Nil {
		spec.TemplateVersionID = spec.CreateRequest.TemplateVersionID
		spec.TemplateVersionExplicit = true
	}
	if strings.TrimSpace(spec.CreateRequest.Name) != "" {
		spec.Name = strings.TrimSpace(spec.CreateRequest.Name)
		spec.NameExplicit = true
	}

	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		// If we could decode into CreateWorkspaceRequest but not into map,
		// keep going with the structured parse.
		return spec, nil
	}

	if value, ok := findValue(payload, "name", "workspace_name", "workspaceName"); ok {
		if name := strings.TrimSpace(toString(value)); name != "" {
			spec.Name = name
			spec.NameExplicit = true
		}
	}

	if value, ok := findValue(payload, "template_id", "templateId", "templateID"); ok {
		if parsedID := parseUUIDValue(value); parsedID != uuid.Nil {
			spec.TemplateID = parsedID
			spec.TemplateExplicit = true
		}
	}
	if value, ok := findValue(payload, "template"); ok {
		templateID, templateRef, explicit := parseTemplateReference(value)
		if explicit {
			spec.TemplateExplicit = true
		}
		if templateID != uuid.Nil {
			spec.TemplateID = templateID
		}
		if templateRef != "" {
			spec.TemplateRef = templateRef
		}
	}
	if value, ok := findValue(payload, "template_name", "templateName"); ok {
		if templateRef := strings.TrimSpace(toString(value)); templateRef != "" {
			spec.TemplateRef = templateRef
			spec.TemplateExplicit = true
		}
	}

	if value, ok := findValue(payload, "template_version_id", "templateVersionID", "templateVersionId"); ok {
		if parsedID := parseUUIDValue(value); parsedID != uuid.Nil {
			spec.TemplateVersionID = parsedID
			spec.TemplateVersionExplicit = true
		}
	}
	if value, ok := findValue(payload, "template_version"); ok {
		templateVersionID, templateVersionRef, explicit := parseTemplateReference(value)
		if explicit {
			spec.TemplateVersionExplicit = true
		}
		if templateVersionID != uuid.Nil {
			spec.TemplateVersionID = templateVersionID
		}
		if templateVersionRef != "" {
			spec.TemplateVersionRef = templateVersionRef
		}
	}
	if value, ok := findValue(payload, "template_version_name", "templateVersionName"); ok {
		if templateVersionRef := strings.TrimSpace(toString(value)); templateVersionRef != "" {
			spec.TemplateVersionRef = templateVersionRef
			spec.TemplateVersionExplicit = true
		}
	}

	return spec, nil
}

// FinalizeWorkspaceName resolves and validates the workspace name from the
// explicit request or prompt/template fallback.
func FinalizeWorkspaceName(
	createReq *codersdk.CreateWorkspaceRequest,
	spec WorkspaceSpec,
	prompt string,
	templateName string,
) error {
	if createReq == nil {
		return xerrors.New("create workspace request is nil")
	}

	name := strings.TrimSpace(createReq.Name)
	explicit := spec.NameExplicit || name != ""

	if name == "" {
		seed := strings.TrimSpace(prompt)
		if seed == "" {
			seed = templateName
		}
		createReq.Name = generatedWorkspaceName(seed)
		return nil
	}

	if err := codersdk.NameValid(name); err != nil {
		if explicit {
			return xerrors.Errorf("workspace name %q is invalid: %w", name, err)
		}
		createReq.Name = generatedWorkspaceName(name)
		return nil
	}

	createReq.Name = name
	return nil
}

// TemplateMatchesReference returns whether a template name/display name matches
// the provided reference.
func TemplateMatchesReference(template database.Template, reference string) bool {
	reference = strings.TrimSpace(strings.ToLower(reference))
	if reference == "" {
		return false
	}
	return strings.EqualFold(template.Name, reference) ||
		strings.EqualFold(strings.TrimSpace(template.DisplayName), reference)
}

// SearchPromptTemplates returns the highest-scored templates for prompt-based
// matching. For empty prompts it returns the input templates as-is.
func SearchPromptTemplates(
	prompt string,
	templates []database.Template,
	limit int,
) []database.Template {
	if len(templates) == 0 {
		return []database.Template{}
	}

	prompt = strings.TrimSpace(strings.ToLower(prompt))
	if prompt == "" {
		return templates
	}

	tokens := promptTokenPattern.FindAllString(prompt, -1)
	type scoredTemplate struct {
		template database.Template
		score    int
	}
	scored := make([]scoredTemplate, 0, len(templates))
	for _, template := range templates {
		score := scoreTemplateAgainstPrompt(prompt, tokens, template)
		if score == 0 {
			continue
		}
		scored = append(scored, scoredTemplate{
			template: template,
			score:    score,
		})
	}
	if len(scored) == 0 {
		return []database.Template{}
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].template.Name != scored[j].template.Name {
			return scored[i].template.Name < scored[j].template.Name
		}
		return scored[i].template.ID.String() < scored[j].template.ID.String()
	})

	if limit <= 0 || limit > len(scored) {
		limit = len(scored)
	}

	candidates := make([]database.Template, 0, limit)
	for i := 0; i < limit; i++ {
		candidates = append(candidates, scored[i].template)
	}
	return candidates
}

func selectTemplateWithModel(
	ctx context.Context,
	model aiapi.LanguageModel,
	prompt string,
	candidates []database.Template,
) (database.Template, error) {
	candidateLines := make([]string, 0, len(candidates))
	for i := range candidates {
		template := candidates[i]
		candidateLines = append(candidateLines, fmt.Sprintf(
			"%d) id=%s name=%q display_name=%q description=%q",
			i+1,
			template.ID,
			template.Name,
			template.DisplayName,
			template.Description,
		))
	}

	modelPrompt := strings.TrimSpace(fmt.Sprintf(
		"User prompt:\n%s\n\nTemplates:\n%s",
		strings.TrimSpace(prompt),
		strings.Join(candidateLines, "\n"),
	))

	response, err := ai.GenerateText(ctx, []aiapi.Message{
		&aiapi.SystemMessage{
			Content: "Select exactly one template for workspace creation. " +
				"Respond only with JSON: {\"template_id\":\"<uuid>\",\"reason\":\"<short reason>\"}. " +
				"Use only candidate IDs listed in the prompt.",
		},
		&aiapi.UserMessage{Content: aiapi.ContentFromText(modelPrompt)},
	}, ai.WithModel(model))
	if err != nil {
		return database.Template{}, newTemplateSelectionReasonError("multiple templates matched and model disambiguation failed")
	}

	selectionID, ok := parseTemplateSelection(response.Content, candidates)
	if !ok {
		return database.Template{}, newTemplateSelectionReasonError("multiple templates matched and model response was ambiguous")
	}

	for i := range candidates {
		if candidates[i].ID == selectionID {
			return candidates[i], nil
		}
	}

	return database.Template{}, newTemplateSelectionReasonError("model selected an unknown template")
}

func parseTemplateSelection(content []aiapi.ContentBlock, candidates []database.Template) (uuid.UUID, bool) {
	text := extractResponseText(content)
	if text == "" {
		return uuid.Nil, false
	}

	candidateByID := make(map[uuid.UUID]struct{}, len(candidates))
	for i := range candidates {
		candidateByID[candidates[i].ID] = struct{}{}
	}

	tryIDs := make([]string, 0, 2)
	if id := parseSelectionIDFromJSON(text); id != "" {
		tryIDs = append(tryIDs, id)
	}

	uuidMatches := templateSelectionUUIDPattern.FindAllString(strings.ToLower(text), -1)
	tryIDs = append(tryIDs, uuidMatches...)

	for _, rawID := range tryIDs {
		parsedID, err := uuid.Parse(strings.TrimSpace(rawID))
		if err != nil {
			continue
		}
		if _, ok := candidateByID[parsedID]; ok {
			return parsedID, true
		}
	}

	// Last resort: if the model only referenced one candidate by name, accept it.
	lower := strings.ToLower(text)
	nameMatches := make([]uuid.UUID, 0, len(candidates))
	for i := range candidates {
		if strings.Contains(lower, strings.ToLower(candidates[i].Name)) {
			nameMatches = append(nameMatches, candidates[i].ID)
			continue
		}
		display := strings.TrimSpace(candidates[i].DisplayName)
		if display != "" && strings.Contains(lower, strings.ToLower(display)) {
			nameMatches = append(nameMatches, candidates[i].ID)
		}
	}
	if len(nameMatches) == 1 {
		return nameMatches[0], true
	}

	return uuid.Nil, false
}

func parseSelectionIDFromJSON(text string) string {
	parse := func(raw string) string {
		var payload struct {
			TemplateID string `json:"template_id"`
			ID         string `json:"id"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return ""
		}
		if strings.TrimSpace(payload.TemplateID) != "" {
			return strings.TrimSpace(payload.TemplateID)
		}
		return strings.TrimSpace(payload.ID)
	}

	if id := parse(text); id != "" {
		return id
	}

	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return parse(text[start : end+1])
	}
	return ""
}

func extractResponseText(content []aiapi.ContentBlock) string {
	var builder strings.Builder
	for _, block := range content {
		switch payload := block.(type) {
		case *aiapi.TextBlock:
			if strings.TrimSpace(payload.Text) == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(payload.Text)
		case *aiapi.ReasoningBlock:
			if strings.TrimSpace(payload.Text) == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(payload.Text)
		}
	}
	return strings.TrimSpace(builder.String())
}

func (c *workspaceCreator) streamWorkspaceBuildLogs(
	ctx context.Context,
	jobID uuid.UUID,
	emit CreateWorkspaceBuildLogHandler,
) {
	if c == nil || c.adapter == nil || emit == nil || jobID == uuid.Nil || c.adapter.Pubsub() == nil {
		return
	}

	streamCtx, cancel := context.WithTimeout(ctx, chatWorkspaceBuildLogStreamTimeout)
	defer cancel()

	after := int64(0)
	streamLogs := func() error {
		logs, err := c.adapter.Database().GetProvisionerLogsAfterID(streamCtx, database.GetProvisionerLogsAfterIDParams{
			JobID:        jobID,
			CreatedAfter: after,
		})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get provisioner logs: %w", err)
		}
		for _, log := range logs {
			emit(CreateWorkspaceBuildLog{
				Source: string(log.Source),
				Level:  string(log.Level),
				Stage:  log.Stage,
				Output: log.Output,
			})
			after = log.ID
		}
		return nil
	}

	if err := streamLogs(); err != nil {
		c.adapter.Logger().Debug(ctx, "failed to stream initial workspace build logs",
			slog.F("job_id", jobID),
			slog.Error(err),
		)
		return
	}

	complete, err := c.isProvisionerJobComplete(streamCtx, jobID)
	if err != nil {
		c.adapter.Logger().Debug(ctx, "failed to check workspace build status",
			slog.F("job_id", jobID),
			slog.Error(err),
		)
		return
	}
	if complete {
		return
	}

	notifications := make(chan provisionersdk.ProvisionerJobLogsNotifyMessage, 64)
	notifyErrors := make(chan error, 1)
	subCancel, err := c.adapter.Pubsub().SubscribeWithErr(
		provisionersdk.ProvisionerJobLogsNotifyChannel(jobID),
		func(_ context.Context, message []byte, subErr error) {
			if subErr != nil {
				select {
				case <-streamCtx.Done():
				case notifyErrors <- subErr:
				default:
				}
				return
			}

			var notification provisionersdk.ProvisionerJobLogsNotifyMessage
			if err := json.Unmarshal(message, &notification); err != nil {
				select {
				case <-streamCtx.Done():
				case notifyErrors <- err:
				default:
				}
				return
			}

			select {
			case <-streamCtx.Done():
			case notifications <- notification:
			default:
			}
		},
	)
	if err != nil {
		c.adapter.Logger().Debug(ctx, "failed to subscribe to workspace build logs",
			slog.F("job_id", jobID),
			slog.Error(err),
		)
		return
	}
	defer subCancel()

	// Re-query logs after subscribing to avoid dropping events between the
	// initial query and pubsub registration.
	if err := streamLogs(); err != nil {
		c.adapter.Logger().Debug(ctx, "failed to stream workspace build logs after subscribe",
			slog.F("job_id", jobID),
			slog.Error(err),
		)
		return
	}

	complete, err = c.isProvisionerJobComplete(streamCtx, jobID)
	if err != nil {
		c.adapter.Logger().Debug(ctx, "failed to check workspace build status after subscribe",
			slog.F("job_id", jobID),
			slog.Error(err),
		)
		return
	}
	if complete {
		return
	}

	pollTicker := time.NewTicker(chatWorkspaceBuildLogPollInterval)
	defer pollTicker.Stop()

	for {
		select {
		case <-streamCtx.Done():
			return
		case subErr := <-notifyErrors:
			c.adapter.Logger().Debug(ctx, "workspace build log subscription ended with error",
				slog.F("job_id", jobID),
				slog.Error(subErr),
			)
			return
		case notification := <-notifications:
			if err := streamLogs(); err != nil {
				c.adapter.Logger().Debug(ctx, "failed to stream workspace build logs from notification",
					slog.F("job_id", jobID),
					slog.Error(err),
				)
				return
			}
			if notification.EndOfLogs {
				return
			}
		case <-pollTicker.C:
			complete, err := c.isProvisionerJobComplete(streamCtx, jobID)
			if err != nil {
				c.adapter.Logger().Debug(ctx, "failed to poll workspace build status",
					slog.F("job_id", jobID),
					slog.Error(err),
				)
				return
			}
			if !complete {
				continue
			}
			if err := streamLogs(); err != nil {
				c.adapter.Logger().Debug(ctx, "failed to stream final workspace build logs",
					slog.F("job_id", jobID),
					slog.Error(err),
				)
			}
			return
		}
	}
}

func (c *workspaceCreator) isProvisionerJobComplete(ctx context.Context, jobID uuid.UUID) (bool, error) {
	job, err := c.adapter.Database().GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		return false, xerrors.Errorf("get provisioner job: %w", err)
	}

	switch job.JobStatus {
	case database.ProvisionerJobStatusCanceled,
		database.ProvisionerJobStatusFailed,
		database.ProvisionerJobStatusSucceeded:
		return true, nil
	default:
		return false, nil
	}
}

func (c *workspaceCreator) resolveTemplateSelection(
	ctx context.Context,
	r *http.Request,
	req CreateWorkspaceToolRequest,
	spec WorkspaceSpec,
	createReq *codersdk.CreateWorkspaceRequest,
) (database.Template, error) {
	var selectedTemplate database.Template

	templateID := createReq.TemplateID
	if templateID == uuid.Nil {
		templateID = spec.TemplateID
	}

	templateVersionID := createReq.TemplateVersionID
	if templateVersionID == uuid.Nil {
		templateVersionID = spec.TemplateVersionID
	}

	templateRef := strings.TrimSpace(spec.TemplateRef)
	templateVersionRef := strings.TrimSpace(spec.TemplateVersionRef)

	if templateVersionID == uuid.Nil && templateVersionRef != "" {
		if parsedID, err := uuid.Parse(templateVersionRef); err == nil {
			templateVersionID = parsedID
			templateVersionRef = ""
		}
	}

	if templateVersionID != uuid.Nil {
		templateVersion, err := c.adapter.Database().GetTemplateVersionByID(ctx, templateVersionID)
		if err != nil {
			return database.Template{}, newTemplateSelectionReasonError("template version was not found or is not accessible")
		}
		selectedTemplate, err = c.adapter.Database().GetTemplateByID(ctx, templateVersion.TemplateID.UUID)
		if err != nil {
			return database.Template{}, xerrors.Errorf("get template for template version: %w", err)
		}
		if templateID != uuid.Nil && templateID != selectedTemplate.ID {
			return database.Template{}, newTemplateSelectionReasonError("template_id does not match template_version_id")
		}
		if templateRef != "" && !TemplateMatchesReference(selectedTemplate, templateRef) {
			return database.Template{}, newTemplateSelectionReasonError("template does not match template_version")
		}

		createReq.TemplateVersionID = templateVersionID
		createReq.TemplateID = uuid.Nil
		return selectedTemplate, nil
	}

	switch {
	case templateID != uuid.Nil:
		template, err := c.adapter.Database().GetTemplateByID(ctx, templateID)
		if err != nil {
			return database.Template{}, newTemplateSelectionReasonError("template was not found or is not accessible")
		}
		selectedTemplate = template
	case templateRef != "":
		template, err := c.resolveTemplateReference(ctx, r, templateRef)
		if err != nil {
			return database.Template{}, err
		}
		selectedTemplate = template
	default:
		candidates, err := c.searchPromptTemplates(ctx, r, req.Prompt)
		if err != nil {
			return database.Template{}, xerrors.Errorf("search templates for prompt: %w", err)
		}
		switch len(candidates) {
		case 0:
			return database.Template{}, newTemplateSelectionReasonError("no authorized templates matched the prompt")
		case 1:
			selectedTemplate = candidates[0]
		default:
			if req.Model == nil {
				return database.Template{}, newTemplateSelectionReasonError("multiple templates matched and no model is available to disambiguate")
			}
			template, err := selectTemplateWithModel(ctx, req.Model, req.Prompt, candidates)
			if err != nil {
				return database.Template{}, err
			}
			selectedTemplate = template
		}
	}

	if templateVersionRef != "" {
		templateVersion, err := c.adapter.Database().GetTemplateVersionByTemplateIDAndName(ctx, database.GetTemplateVersionByTemplateIDAndNameParams{
			TemplateID: uuid.NullUUID{
				UUID:  selectedTemplate.ID,
				Valid: true,
			},
			Name: templateVersionRef,
		})
		if err != nil {
			return database.Template{}, newTemplateSelectionReasonError("template version was not found for the selected template")
		}
		createReq.TemplateVersionID = templateVersion.ID
		createReq.TemplateID = uuid.Nil
		return selectedTemplate, nil
	}

	createReq.TemplateID = selectedTemplate.ID
	createReq.TemplateVersionID = uuid.Nil
	return selectedTemplate, nil
}

func (c *workspaceCreator) resolveTemplateReference(
	ctx context.Context,
	r *http.Request,
	templateRef string,
) (database.Template, error) {
	templateRef = strings.TrimSpace(templateRef)
	if templateRef == "" {
		return database.Template{}, newTemplateSelectionReasonError("template reference is empty")
	}

	if parsedID, err := uuid.Parse(templateRef); err == nil {
		template, err := c.adapter.Database().GetTemplateByID(ctx, parsedID)
		if err != nil {
			return database.Template{}, newTemplateSelectionReasonError("template was not found or is not accessible")
		}
		return template, nil
	}

	templates, err := c.authorizedTemplates(ctx, r)
	if err != nil {
		return database.Template{}, err
	}

	matches := make([]database.Template, 0, len(templates))
	for _, template := range templates {
		if TemplateMatchesReference(template, templateRef) {
			matches = append(matches, template)
		}
	}

	switch len(matches) {
	case 0:
		return database.Template{}, newTemplateSelectionReasonError(fmt.Sprintf("no authorized template matched %q", templateRef))
	case 1:
		return matches[0], nil
	default:
		return database.Template{}, newTemplateSelectionReasonError(fmt.Sprintf("multiple templates matched %q; provide template_id", templateRef))
	}
}

func (c *workspaceCreator) searchPromptTemplates(
	ctx context.Context,
	r *http.Request,
	prompt string,
) ([]database.Template, error) {
	templates, err := c.authorizedTemplates(ctx, r)
	if err != nil {
		return nil, err
	}
	return SearchPromptTemplates(prompt, templates, 20), nil
}

func (c *workspaceCreator) authorizedTemplates(
	ctx context.Context,
	r *http.Request,
) ([]database.Template, error) {
	templates, err := c.adapter.AuthorizedTemplates(ctx, r)
	if err != nil {
		return nil, err
	}
	if len(templates) == 0 {
		return []database.Template{}, nil
	}
	return templates, nil
}

func generatedWorkspaceName(seed string) string {
	base := codersdk.UsernameFrom(strings.TrimSpace(strings.ToLower(seed)))
	if strings.TrimSpace(base) == "" {
		base = "workspace"
	}

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:4]
	if len(base) > 27 {
		base = strings.Trim(base[:27], "-")
	}
	if base == "" {
		base = "workspace"
	}

	name := fmt.Sprintf("%s-%s", base, suffix)
	if err := codersdk.NameValid(name); err == nil {
		return name
	}
	return namesgenerator.NameDigitWith("-")
}

func scoreTemplateAgainstPrompt(prompt string, tokens []string, template database.Template) int {
	name := strings.ToLower(template.Name)
	display := strings.ToLower(strings.TrimSpace(template.DisplayName))
	description := strings.ToLower(strings.TrimSpace(template.Description))

	score := 0
	if strings.Contains(prompt, name) {
		score += 6
	}
	if display != "" && strings.Contains(prompt, display) {
		score += 5
	}

	for _, token := range tokens {
		if len(token) < 3 {
			continue
		}
		switch {
		case strings.Contains(name, token):
			score += 3
		case display != "" && strings.Contains(display, token):
			score += 2
		case description != "" && strings.Contains(description, token):
			score++
		}
	}

	return score
}

func parseTemplateReference(value any) (uuid.UUID, string, bool) {
	switch raw := value.(type) {
	case nil:
		return uuid.Nil, "", false
	case string:
		text := strings.TrimSpace(raw)
		if text == "" {
			return uuid.Nil, "", false
		}
		if parsedID, err := uuid.Parse(text); err == nil {
			return parsedID, "", true
		}
		return uuid.Nil, text, true
	case map[string]any:
		idValue, _ := findValue(raw, "id", "template_id", "template_version_id")
		nameValue, _ := findValue(raw, "name", "display_name", "template_name", "template_version_name")

		parsedID := parseUUIDValue(idValue)
		name := strings.TrimSpace(toString(nameValue))
		return parsedID, name, parsedID != uuid.Nil || name != ""
	default:
		text := strings.TrimSpace(toString(value))
		if text == "" {
			return uuid.Nil, "", false
		}
		if parsedID, err := uuid.Parse(text); err == nil {
			return parsedID, "", true
		}
		return uuid.Nil, text, true
	}
}

func parseUUIDValue(value any) uuid.UUID {
	text := strings.TrimSpace(toString(value))
	if text == "" {
		return uuid.Nil
	}
	parsedID, err := uuid.Parse(text)
	if err != nil {
		return uuid.Nil
	}
	return parsedID
}

func findValue(values map[string]any, keys ...string) (any, bool) {
	if len(values) == 0 {
		return nil, false
	}

	targets := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		targets[normalizeKey(key)] = struct{}{}
	}

	for key, value := range values {
		if _, ok := targets[normalizeKey(key)]; ok {
			return value, true
		}
	}
	return nil, false
}

func normalizeKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return key
}

func toString(value any) string {
	switch raw := value.(type) {
	case nil:
		return ""
	case string:
		return raw
	case json.Number:
		return raw.String()
	default:
		return fmt.Sprintf("%v", value)
	}
}
