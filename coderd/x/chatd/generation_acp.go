package chatd

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatacp"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

const (
	// acpWorkspaceReadyTimeout bounds workspace startup, agent
	// dialing, and adapter readiness for a turn.
	acpWorkspaceReadyTimeout = 10 * time.Minute
	// acpWorkspacePollInterval paces build and dial polling.
	acpWorkspacePollInterval = 3 * time.Second
	// acpPreflightTimeout bounds the adapter binary check.
	acpPreflightTimeout = 30 * time.Second
	// acpPersistStateTimeout bounds the best-effort
	// runtime_state write after a turn.
	acpPersistStateTimeout = 15 * time.Second
)

// startACPGeneration is re-entrant because the runner redispatches
// after commits. If output is already committed, it finishes the turn
// instead of prompting again.
func (s *taskStarter) startACPGeneration(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	harness chatacp.Harness,
	chat database.Chat,
	history []database.ChatMessage,
) error {
	turn, err := acpTurnFromHistory(ctx, s.opts.Logger, harness, history)
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
	}
	if !turn.generate {
		return s.finishGenerationTurn(ctx, machine, input, generationDecision{
			kind:         generationActionFinishTurn,
			finishReason: generationFinishReasonComplete,
		}, generationAttemptNotRequired)
	}

	cfg, err := s.server.acpTurnConfig(ctx, harness, chat, turn.modelConfigID)
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
	}

	readinessDeadline := s.opts.Clock.Now("chatworker", "chatacp-readiness").Add(acpWorkspaceReadyTimeout)
	if err := s.ensureACPWorkspaceRunning(ctx, harness, chat, readinessDeadline); err != nil {
		if ctx.Err() != nil {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("ensure workspace running: %w", err))
		}
		return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
	}

	currentChat := chat
	var chatStateMu sync.Mutex
	workspaceCtx := turnWorkspaceContext{
		server:      s.server,
		chatStateMu: &chatStateMu,
		currentChat: &currentChat,
		loadChatSnapshot: func(loadCtx context.Context, chatID uuid.UUID) (database.Chat, error) {
			return s.server.db.GetChatByID(loadCtx, chatID)
		},
	}
	defer workspaceCtx.close()

	conn, agent, err := s.dialACPAgent(ctx, &workspaceCtx, readinessDeadline)
	if err != nil {
		if ctx.Err() != nil {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("dial workspace agent: %w", err))
		}
		return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
	}

	env := harness.Env(cfg.TurnCredentials)

	transportFn := s.server.acpTransportFn
	if transportFn == nil {
		transportFn = s.sshACPTransport
	}
	transport, closeTransport, err := transportFn(ctx, conn, agent, harness, env, readinessDeadline)
	if err != nil {
		if ctx.Err() != nil {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("acp transport: %w", err))
		}
		return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
	}
	defer closeTransport()

	state := chatacp.ParseRuntimeState(chat.RuntimeState.RawMessage)
	cwd := agent.ExpandedDirectory
	if state.SessionID == "" {
		cwd, err = acpNewSessionCwd(ctx, s.opts.Clock, agent, readinessDeadline, func(ctx context.Context) (database.WorkspaceAgent, error) {
			return s.server.db.GetWorkspaceAgentByID(ctx, agent.ID)
		})
		if err != nil {
			if ctx.Err() != nil {
				return errors.Join(errTaskExpectedExit, xerrors.Errorf("resolve session directory: %w", err))
			}
			return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
		}
	}
	if cwd == "" {
		cwd, err = chattool.ResolveWorkspaceHome(ctx, conn)
		if err != nil {
			return s.finishGenerationError(ctx, machine, input, xerrors.Errorf("resolve workspace home: %w", err), generationAttemptNotRequired)
		}
	}

	attempt, err := s.beginGenerationAttempt(ctx, machine, input)
	if err != nil {
		return xerrors.Errorf("begin generation attempt: %w", err)
	}
	defer attempt.closeEpisode()

	startedAt := s.opts.Clock.Now("chatworker", "chatacp")
	outcome, runErr := chatacp.RunTurn(ctx, transport, chatacp.TurnInput{
		SessionID:      state.SessionID,
		SessionCwd:     state.Cwd,
		Cwd:            cwd,
		PromptText:     turn.prompt,
		ReseedContext:  chatacp.BuildReseedContext(turn.reseed),
		PermissionMode: cfg.permissionMode,
		AgentName:      harness.DisplayName,
		Publish:        attempt.publish,
		Logger:         s.opts.Logger,
		Clock:          s.opts.Clock,
	})
	elapsed := s.opts.Clock.Now("chatworker", "chatacp").Sub(startedAt)

	next, turnUsage := state.Advance(outcome, cwd, s.opts.Clock.Now("chatworker", "chatacp"))

	// Record the session even when the turn was interrupted or the
	// commit below fails: the workspace-side session advanced either
	// way, and the next turn should resume it.
	if outcome.SessionID != "" {
		s.persistACPRuntimeState(ctx, chat.ID, state, next)
	}

	if ctx.Err() != nil {
		return errors.Join(errTaskExpectedExit, xerrors.Errorf("acp turn interrupted: %w", ctx.Err()))
	}
	if runErr != nil {
		if modeErr, ok := errors.AsType[*chatacp.SessionModeError](runErr); ok {
			runErr = chaterror.WithClassification(runErr, chaterror.ClassifiedError{
				Kind: codersdk.ChatErrorKindConfig,
				Message: fmt.Sprintf("The %s runtime's permission mode %q is not supported by the adapter; ask an administrator to change it in the runtime config.",
					harness.DisplayName, modeErr.Mode),
			})
		}
		return s.finishGenerationError(ctx, machine, input, runErr, requireGenerationAttempt(attempt.number))
	}
	if len(outcome.Content) == 0 {
		return s.finishGenerationTurn(ctx, machine, input, generationDecision{
			kind:         generationActionFinishTurn,
			finishReason: generationFinishReasonComplete,
		}, requireGenerationAttempt(attempt.number))
	}

	messages, err := buildCommitStepMessages(buildCommitStepMessagesInput{
		step: stepData{
			Content: outcome.Content,
			Usage:   turnUsage,
			Runtime: elapsed,
		},
		// The applied selection groups per-model token analytics.
		// modelCallConfig stays zero, so these turns intentionally
		// carry no cost.
		modelConfigID:  cfg.modelConfigID,
		logger:         s.opts.Logger,
		contentVersion: chatprompt.CurrentContentVersion,
	})
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, err, requireGenerationAttempt(attempt.number))
	}
	return s.commitGenerationStep(ctx, machine, input, attempt.number, generationActionGenerateAssistant, messages, generationCommitHooks{})
}

type acpTurn struct {
	generate bool
	prompt   string
	reseed   []chatacp.ReseedTurn
	// modelConfigID is the explicit model selection carried by the
	// newest triggering user message. Zero means the runtime default
	// chain (admin pin, then adapter default).
	modelConfigID uuid.UUID
}

// acpTurnFromHistory derives the ACP prompt for this turn from
// persisted history. The prompt is the trailing run of user messages
// (multiple when earlier turns failed before generating a reply);
// everything before it becomes reseed context for the lossy
// new-session fallback. generate is false when history ends with
// assistant or tool output, meaning the turn already generated and
// only FinishTurn remains. System rows (hook notices) are never model
// input: they neither end the trailing run nor reach the adapter.
func acpTurnFromHistory(ctx context.Context, logger slog.Logger, harness chatacp.Harness, history []database.ChatMessage) (acpTurn, error) {
	firstTrailingUser := len(history)
scan:
	for i := len(history) - 1; i >= 0; i-- {
		switch history[i].Role {
		case database.ChatMessageRoleUser:
			firstTrailingUser = i
		case database.ChatMessageRoleSystem:
		default:
			break scan
		}
	}
	if firstTrailingUser == len(history) {
		return acpTurn{}, nil
	}

	var prompt strings.Builder
	var modelConfigID uuid.UUID
	for _, msg := range history[firstTrailingUser:] {
		if msg.Role != database.ChatMessageRoleUser {
			continue
		}
		text, err := chatMessageText(msg)
		if err != nil {
			return acpTurn{}, xerrors.Errorf("parse user message %d: %w", msg.ID, err)
		}
		// The newest trailing user message carries the model selection
		// for this turn; picks on earlier queued messages are superseded.
		modelConfigID = msg.ModelConfigID.UUID
		if text == "" {
			continue
		}
		if prompt.Len() > 0 {
			_, _ = prompt.WriteString("\n\n")
		}
		_, _ = prompt.WriteString(text)
	}
	if strings.TrimSpace(prompt.String()) == "" {
		return acpTurn{}, chaterror.WithClassification(
			xerrors.New("user message has no text content"),
			chaterror.ClassifiedError{
				Kind:    codersdk.ChatErrorKindGeneric,
				Message: harness.DisplayName + " chats currently support text messages only.",
			},
		)
	}

	var reseed []chatacp.ReseedTurn
	for _, msg := range history[:firstTrailingUser] {
		var role string
		switch msg.Role {
		case database.ChatMessageRoleUser:
			role = "User"
		case database.ChatMessageRoleAssistant:
			role = "Assistant"
		default:
			continue
		}
		text, err := chatMessageText(msg)
		if err != nil {
			// Reseed is lossy by design; skip entries that fail to
			// parse instead of failing the turn.
			logger.Debug(ctx, "skip reseed message", slog.F("message_id", msg.ID), slog.Error(err))
			continue
		}
		if text == "" {
			continue
		}
		reseed = append(reseed, chatacp.ReseedTurn{Role: role, Text: text})
	}

	return acpTurn{
		generate:      true,
		prompt:        prompt.String(),
		reseed:        reseed,
		modelConfigID: modelConfigID,
	}, nil
}

// chatMessageText joins the text parts of a persisted message.
func chatMessageText(msg database.ChatMessage) (string, error) {
	parts, err := chatprompt.ParseContent(msg)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, part := range parts {
		if part.Type != codersdk.ChatMessagePartTypeText || part.Text == "" {
			continue
		}
		if sb.Len() > 0 {
			_, _ = sb.WriteString("\n\n")
		}
		_, _ = sb.WriteString(part.Text)
	}
	return sb.String(), nil
}

type acpTurnConfig struct {
	chatacp.TurnCredentials
	permissionMode string
	// modelConfigID is the applied explicit selection, stamped on the
	// turn's assistant messages. Zero when the runtime default chain
	// (admin pin, then adapter default) applied.
	modelConfigID uuid.UUID
}

// acpTurnConfig loads the organization's runtime config and a
// deployment key for the harness provider for one turn. The key is
// injected into the adapter's process environment and never written to
// workspace disk. A non-zero selection (the triggering user message's
// model config) overrides the admin model pin and sources credentials
// from the selected config's provider.
func (p *Server) acpTurnConfig(ctx context.Context, harness chatacp.Harness, chat database.Chat, selection uuid.UUID) (acpTurnConfig, error) {
	cfg, err := p.db.GetChatRuntimeConfig(ctx, database.GetChatRuntimeConfigParams{
		OrganizationID: chat.OrganizationID,
		Runtime:        chat.Runtime,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return acpTurnConfig{}, chaterror.WithClassification(
				xerrors.New("chat runtime config missing"),
				chaterror.ClassifiedError{
					Kind:    codersdk.ChatErrorKindConfig,
					Message: fmt.Sprintf("The %s runtime is not configured for this organization.", harness.DisplayName),
				},
			)
		}
		return acpTurnConfig{}, xerrors.Errorf("get chat runtime config: %w", err)
	}
	if !cfg.Enabled {
		return acpTurnConfig{}, chaterror.WithClassification(
			xerrors.New("chat runtime config disabled"),
			chaterror.ClassifiedError{
				Kind:    codersdk.ChatErrorKindProviderDisabled,
				Message: fmt.Sprintf("The %s runtime is disabled for this organization.", harness.DisplayName),
			},
		)
	}

	providers, err := p.db.GetAIProviders(ctx, database.GetAIProvidersParams{})
	if err != nil {
		return acpTurnConfig{}, xerrors.Errorf("get ai providers: %w", err)
	}
	harnessProviders := make([]database.AIProvider, 0, len(providers))
	for _, provider := range providers {
		if provider.Type == database.AIProviderType(harness.ProviderType) {
			harnessProviders = append(harnessProviders, provider)
		}
	}
	configuredProviders, err := p.aiProviderConfigs(ctx, harnessProviders)
	if err != nil {
		return acpTurnConfig{}, xerrors.Errorf("configure %s providers: %w", harness.ProviderType, err)
	}
	credsByProvider := make(map[uuid.UUID]chatacp.TurnCredentials, len(configuredProviders))
	for _, configured := range configuredProviders {
		if configured.APIKey == "" {
			continue
		}
		credsByProvider[configured.ProviderID] = chatacp.TurnCredentials{APIKey: configured.APIKey, BaseURL: configured.BaseURL}
	}
	if len(credsByProvider) == 0 {
		return acpTurnConfig{}, chaterror.WithClassification(
			xerrors.Errorf("no %s provider key configured", harness.ProviderType),
			chaterror.ClassifiedError{
				Kind: codersdk.ChatErrorKindMissingKey,
				Message: fmt.Sprintf("%s requires a deployment %s API key. An administrator must configure the %s AI provider.",
					harness.DisplayName, harness.ProviderLabel, harness.ProviderLabel),
			},
		)
	}

	out := acpTurnConfig{
		permissionMode: cmp.Or(cfg.PermissionMode, harness.DefaultSessionMode),
	}
	if selection != uuid.Nil {
		modelCtx, err := p.callerModelConfigContext(ctx, chat.OwnerID)
		if err != nil {
			return acpTurnConfig{}, err
		}
		modelConfig, provider, err := chatstate.FetchACPModelConfig(modelCtx, p.db, chat.OrganizationID, harness, selection)
		if err == nil {
			if creds, ok := credsByProvider[provider.ID]; ok {
				creds.Model = modelConfig.Model
				out.TurnCredentials = creds
				out.modelConfigID = selection
				return out, nil
			}
			err = xerrors.Errorf("provider %s has no usable key", provider.ID)
		}
		// The selection was valid at send time; losing it mid-flight (config
		// deleted or disabled, provider changed or left without a key) falls
		// back to the runtime default chain and leaves the assistant messages
		// unstamped. A model is never paired with another provider's key.
		p.logger.Warn(ctx, "acp turn: selected model config unavailable, using runtime default",
			slog.F("chat_id", chat.ID), slog.F("model_config_id", selection), slog.Error(err))
	}
	creds, err := p.acpDefaultCredentials(ctx, harness, chat.OrganizationID, cfg.Model, credsByProvider)
	if err != nil {
		return acpTurnConfig{}, err
	}
	out.TurnCredentials = creds
	return out, nil
}

// acpDefaultCredentials resolves the runtime default chain: the admin
// model pin sources credentials from its own model config's provider,
// and without a pin the single keyed harness provider supplies them
// with the adapter's default model. The chain never guesses between
// providers and never routes a pin whose model config has since been
// disabled or deleted.
func (p *Server) acpDefaultCredentials(
	ctx context.Context,
	harness chatacp.Harness,
	organizationID uuid.UUID,
	pinnedModel string,
	credsByProvider map[uuid.UUID]chatacp.TurnCredentials,
) (chatacp.TurnCredentials, error) {
	candidates := credsByProvider
	if pinnedModel != "" {
		configs, err := p.db.GetEnabledChatModelConfigsByOrganization(ctx, organizationID)
		if err != nil {
			return chatacp.TurnCredentials{}, xerrors.Errorf("get enabled model configs: %w", err)
		}
		pinned := make(map[uuid.UUID]chatacp.TurnCredentials)
		for _, row := range configs {
			providerID := row.ChatModelConfig.AIProviderID.UUID
			if row.Provider != string(harness.ProviderType) || row.ChatModelConfig.Model != pinnedModel {
				continue
			}
			if creds, ok := credsByProvider[providerID]; ok {
				pinned[providerID] = creds
			}
		}
		if len(pinned) == 0 {
			return chatacp.TurnCredentials{}, chaterror.WithClassification(
				xerrors.Errorf("pinned model %q has no enabled %s model config", pinnedModel, harness.ProviderType),
				chaterror.ClassifiedError{
					Kind: codersdk.ChatErrorKindConfig,
					Message: fmt.Sprintf("The %s runtime's pinned model %q is disabled or no longer available; an administrator must update the runtime configuration.",
						harness.DisplayName, pinnedModel),
				},
			)
		}
		candidates = pinned
	}
	if len(candidates) > 1 {
		return chatacp.TurnCredentials{}, chaterror.WithClassification(
			xerrors.Errorf("%d %s providers are keyed", len(candidates), harness.ProviderType),
			chaterror.ClassifiedError{
				Kind: codersdk.ChatErrorKindConfig,
				Message: fmt.Sprintf("Multiple %s providers are enabled, so the %s runtime cannot choose one. Select a model, or have an administrator keep a single %s provider enabled.",
					harness.ProviderLabel, harness.DisplayName, harness.ProviderLabel),
			},
		)
	}
	var creds chatacp.TurnCredentials
	for _, candidate := range candidates {
		creds = candidate
	}
	creds.Model = pinnedModel
	return creds, nil
}

// ensureACPWorkspaceRunning makes sure the chat's bound
// workspace has a succeeded start build, creating one as the chat
// owner when the workspace is stopped. Agent reachability is handled
// by the dial loop afterwards.
func (s *taskStarter) ensureACPWorkspaceRunning(ctx context.Context, harness chatacp.Harness, chat database.Chat, deadline time.Time) error {
	if !chat.WorkspaceID.Valid {
		return chaterror.WithClassification(
			xerrors.New("runtime chat has no bound workspace"),
			chaterror.ClassifiedError{
				Kind:    codersdk.ChatErrorKindConfig,
				Message: fmt.Sprintf("This chat has no workspace bound to it, so the %s runtime cannot run.", harness.DisplayName),
			},
		)
	}
	db := s.server.db
	workspace, err := db.GetWorkspaceByID(ctx, chat.WorkspaceID.UUID)
	if err != nil {
		return xerrors.Errorf("get workspace: %w", err)
	}

	deletedErr := chaterror.WithClassification(
		xerrors.New("chat workspace deleted"),
		chaterror.ClassifiedError{
			Kind:    codersdk.ChatErrorKindConfig,
			Message: "The workspace backing this chat was deleted, so the conversation cannot continue.",
		},
	)
	if workspace.Deleted {
		return deletedErr
	}

	started := false
	for {
		build, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
		if err != nil {
			return xerrors.Errorf("get latest workspace build: %w", err)
		}
		job, err := db.GetProvisionerJobByID(ctx, build.JobID)
		if err != nil {
			return xerrors.Errorf("get workspace build job: %w", err)
		}
		switch {
		case build.Transition == database.WorkspaceTransitionDelete:
			return deletedErr
		case job.JobStatus == database.ProvisionerJobStatusPending || job.JobStatus == database.ProvisionerJobStatusRunning:
		case build.Transition == database.WorkspaceTransitionStart && job.JobStatus == database.ProvisionerJobStatusSucceeded:
			return nil
		default:
			if started {
				return chaterror.WithClassification(
					xerrors.New("workspace start build did not succeed"),
					chaterror.ClassifiedError{
						Kind:    codersdk.ChatErrorKindGeneric,
						Message: "The workspace backing this chat failed to start. Check the workspace build logs.",
					},
				)
			}
			if s.server.startWorkspaceFn == nil {
				return xerrors.New("workspace starting is not configured")
			}
			s.opts.Logger.Info(ctx, "starting stopped workspace for acp chat",
				slog.F("chat_id", chat.ID), slog.F("workspace_id", workspace.ID))
			if _, err := s.server.startWorkspaceFn(ctx, chat.OwnerID, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
			}); err != nil {
				return chaterror.WithClassification(
					xerrors.Errorf("start workspace: %w", err),
					chaterror.ClassifiedError{
						Kind:    codersdk.ChatErrorKindGeneric,
						Message: "The workspace backing this chat could not be started.",
					},
				)
			}
			started = true
		}
		if !s.opts.Clock.Now("chatworker", "chatacp-workspace").Before(deadline) {
			return chaterror.WithClassification(
				xerrors.New("timed out waiting for workspace to start"),
				chaterror.ClassifiedError{
					Kind:    codersdk.ChatErrorKindTimeout,
					Message: "Timed out waiting for the workspace backing this chat to start.",
				},
			)
		}
		timer := s.opts.Clock.NewTimer(acpWorkspacePollInterval, "chatworker", "chatacp-workspace")
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return xerrors.Errorf("wait for workspace: %w", ctx.Err())
		}
	}
}

// dialACPAgent dials the chat's workspace agent, retrying while
// the agent connects after a workspace start. The turnWorkspaceContext
// handles agent selection, chat binding persistence, and lazy
// validation.
func (s *taskStarter) dialACPAgent(
	ctx context.Context,
	workspaceCtx *turnWorkspaceContext,
	deadline time.Time,
) (workspacesdk.AgentConn, database.WorkspaceAgent, error) {
	for {
		conn, dialErr := workspaceCtx.getWorkspaceConn(ctx)
		if dialErr == nil {
			_, agent, err := workspaceCtx.ensureWorkspaceAgent(ctx)
			if err != nil {
				return nil, database.WorkspaceAgent{}, xerrors.Errorf("resolve workspace agent: %w", err)
			}
			return conn, agent, nil
		}
		if ctx.Err() != nil {
			return nil, database.WorkspaceAgent{}, xerrors.Errorf("dial workspace agent: %w", errors.Join(dialErr, ctx.Err()))
		}
		if !s.opts.Clock.Now("chatworker", "chatacp-dial").Before(deadline) {
			return nil, database.WorkspaceAgent{}, chaterror.WithClassification(
				xerrors.Errorf("dial workspace agent: %w", dialErr),
				chaterror.ClassifiedError{
					Kind:    codersdk.ChatErrorKindTimeout,
					Message: "Timed out waiting for the workspace agent to become reachable.",
				},
			)
		}
		timer := s.opts.Clock.NewTimer(acpWorkspacePollInterval, "chatworker", "chatacp-dial")
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, database.WorkspaceAgent{}, xerrors.Errorf("dial workspace agent: %w", ctx.Err())
		}
	}
}

// ACPTransportFunc builds the ACP transport for one turn from
// an established workspace agent connection. It exists as a seam so
// tests can substitute an in-memory transport; production uses
// sshACPTransport.
type ACPTransportFunc func(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	agent database.WorkspaceAgent,
	harness chatacp.Harness,
	env map[string]string,
	readinessDeadline time.Time,
) (chatacp.Transport, func(), error)

// sshACPTransport opens an SSH client to the workspace agent,
// verifies the adapter binary exists, and returns the non-PTY SSH exec
// transport the ACP session runs over.
func (s *taskStarter) sshACPTransport(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	agent database.WorkspaceAgent,
	harness chatacp.Harness,
	env map[string]string,
	readinessDeadline time.Time,
) (chatacp.Transport, func(), error) {
	sshClient, err := conn.SSHClient(ctx)
	if err != nil {
		return nil, nil, xerrors.Errorf("workspace ssh client: %w", err)
	}
	if err := s.acpAdapterPreflight(ctx, sshClient, agent.ID, harness, readinessDeadline); err != nil {
		_ = sshClient.Close()
		return nil, nil, err
	}
	return &chatacp.SSHTransport{
			Client:  sshClient,
			Command: harness.Command,
			Env:     env,
		}, func() {
			_ = sshClient.Close()
		}, nil
}

// acpAdapterPreflight verifies the adapter binary exists inside
// the workspace before starting a turn, so a template that does not
// ship it produces a legible configuration error instead of an opaque
// protocol failure.
func (s *taskStarter) acpAdapterPreflight(
	ctx context.Context,
	client *gossh.Client,
	agentID uuid.UUID,
	harness chatacp.Harness,
	readinessDeadline time.Time,
) error {
	probe := func(ctx context.Context) error {
		return probeACPAdapter(ctx, s.opts.Clock, client, harness.Command)
	}
	scriptsSettled := func(ctx context.Context) bool {
		agent, err := s.server.db.GetWorkspaceAgentByID(ctx, agentID)
		if err != nil {
			return false
		}
		switch agent.LifecycleState {
		case database.WorkspaceAgentLifecycleStateCreated,
			database.WorkspaceAgentLifecycleStateStarting:
			return false
		default:
			return true
		}
	}
	return waitForACPAdapter(ctx, s.opts.Clock, harness, readinessDeadline, probe, scriptsSettled)
}

// waitForACPAdapter probes for the adapter binary until it
// appears. Templates commonly install the adapter from a startup
// script, and a turn dials the agent as soon as it connects, which can
// be before those scripts finish (always right after an automatic
// workspace start when the install lives outside a persistent volume).
// A failed probe is therefore conclusive only once the agent reports
// its startup scripts settled, or after the ready timeout.
func waitForACPAdapter(
	ctx context.Context,
	clock quartz.Clock,
	harness chatacp.Harness,
	deadline time.Time,
	probe func(context.Context) error,
	scriptsSettled func(context.Context) bool,
) error {
	for {
		// Snapshot settledness before probing: scripts finishing after
		// a failed probe must not fail the turn without a re-probe.
		settled := scriptsSettled(ctx)
		err := probe(ctx)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return xerrors.Errorf("acp adapter preflight: %w", ctx.Err())
		}
		if settled || !clock.Now("chatworker", "chatacp-preflight").Before(deadline) {
			return chaterror.WithClassification(
				xerrors.Errorf("acp adapter preflight: %w", err),
				chaterror.ClassifiedError{
					Kind: codersdk.ChatErrorKindConfig,
					Message: fmt.Sprintf("The workspace does not provide the %s adapter (%s). "+
						"The template configured for this runtime must preinstall it.", harness.DisplayName, harness.Command),
				},
			)
		}
		timer := clock.NewTimer(acpWorkspacePollInterval, "chatworker", "chatacp-preflight")
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return xerrors.Errorf("acp adapter preflight: %w", ctx.Err())
		}
	}
}

// acpNewSessionCwd returns the agent's expanded directory for a new
// ACP session. The directory is persisted with the session and keys
// Claude Code's session storage for the life of the chat, and a turn
// that dials right after a workspace build can see the agent before it
// has reported the expansion. An agent with a configured directory is
// therefore re-read until the expansion lands or the deadline passes;
// an empty result leaves the caller to fall back to the home directory.
func acpNewSessionCwd(
	ctx context.Context,
	clock quartz.Clock,
	agent database.WorkspaceAgent,
	deadline time.Time,
	reload func(context.Context) (database.WorkspaceAgent, error),
) (string, error) {
	for agent.ExpandedDirectory == "" && agent.Directory != "" {
		if !clock.Now("chatworker", "chatacp-cwd").Before(deadline) {
			break
		}
		timer := clock.NewTimer(acpWorkspacePollInterval, "chatworker", "chatacp-cwd")
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return "", xerrors.Errorf("wait for agent directory: %w", ctx.Err())
		}
		refreshed, err := reload(ctx)
		if err != nil {
			return "", xerrors.Errorf("reload workspace agent: %w", err)
		}
		agent = refreshed
	}
	return agent.ExpandedDirectory, nil
}

// probeACPAdapter checks once whether the adapter binary is on
// PATH inside the workspace.
func probeACPAdapter(ctx context.Context, clock quartz.Clock, client *gossh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return xerrors.Errorf("new ssh session: %w", err)
	}
	defer session.Close()

	done := make(chan error, 1)
	go func() {
		done <- session.Run("command -v " + command)
	}()
	timer := clock.NewTimer(acpPreflightTimeout, "chatworker", "chatacp-preflight")
	defer timer.Stop()
	select {
	case err = <-done:
	case <-timer.C:
		return xerrors.New("acp adapter probe timed out")
	case <-ctx.Done():
		return xerrors.Errorf("acp adapter probe: %w", ctx.Err())
	}
	return err
}

// persistACPRuntimeState best-effort records the ACP session
// that served the turn so the next turn can resume it. It runs even
// when the turn was interrupted or the commit fails, because the
// workspace-side session advanced regardless. It skips the write when
// the stored state moved past the one the turn started from: a message
// edit resets the session mid-turn, and recording this turn's session
// would resurrect the transcript the edit discarded.
func (s *taskStarter) persistACPRuntimeState(ctx context.Context, chatID uuid.UUID, observed, next chatacp.RuntimeState) {
	encoded, err := json.Marshal(next)
	if err != nil {
		s.opts.Logger.Warn(ctx, "marshal acp runtime state", slog.F("chat_id", chatID), slog.Error(err))
		return
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), acpPersistStateTimeout)
	defer cancel()
	rows, err := s.opts.Store.UpdateChatRuntimeState(persistCtx, database.UpdateChatRuntimeStateParams{
		ID:                chatID,
		RuntimeState:      pqtype.NullRawMessage{RawMessage: encoded, Valid: true},
		ExpectedUpdatedAt: sql.NullString{String: observed.UpdatedAtText(), Valid: true},
	})
	if err != nil {
		s.opts.Logger.Warn(persistCtx, "persist acp runtime state", slog.F("chat_id", chatID), slog.Error(err))
		return
	}
	if rows == 0 {
		s.opts.Logger.Info(persistCtx, "acp runtime state changed during turn, not recording session",
			slog.F("chat_id", chatID), slog.F("session_id", next.SessionID))
	}
}
