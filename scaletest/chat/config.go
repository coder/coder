package chat

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/scaletest/chatcontrol"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
)

const (
	// MaxToolCallStepsPerTurn leaves room for the final text completion within
	// chatd's per-turn max-step budget.
	MaxToolCallStepsPerTurn = 1199
)

type Config struct {
	// RunID identifies a single CLI invocation across all chat runners.
	RunID string `json:"run_id"`

	// WorkspaceID is the pre-existing workspace to use when the runner should not
	// create its own workspace.
	WorkspaceID uuid.UUID `json:"workspace_id,omitempty"`

	// Workspace is the per-runner workspace configuration used when the runner
	// should create its own workspace before entering the chat start barrier.
	Workspace workspacebuild.Config `json:"workspace,omitempty"`

	// Prompt is the text content for the initial chat message.
	Prompt string `json:"prompt"`

	// ModelConfigID optionally selects a specific model config.
	// When nil the server uses its deployment default.
	ModelConfigID *uuid.UUID `json:"model_config_id,omitempty"`

	// Turns is the total number of user→assistant exchanges per chat.
	// Must be at least 1.
	Turns int `json:"turns"`

	// ToolCallsPerChat is the total number of mock tool-call rounds to
	// distribute across the chat's turns. Turns already controls the number of
	// assistant responses; tool calls add extra model invocations within a turn.
	ToolCallsPerChat int `json:"tool_calls_per_chat,omitempty"`

	// ToolCallSeed is the deterministic seed used to derive the mock's per-chat
	// tool-call distribution.
	ToolCallSeed int64 `json:"tool_call_seed,omitempty"`

	// ToolCallTool is the tool name used when the mock emits a tool call.
	ToolCallTool string `json:"tool_call_tool,omitempty"`

	// ToolCallCommand is the harmless execute command used when the mock emits a
	// tool call.
	ToolCallCommand string `json:"tool_call_command,omitempty"`

	// FollowUpPrompt is the text content for turns 2..N.
	FollowUpPrompt string `json:"follow_up_prompt"`

	// FollowUpStartDelay is the shared delay between the first completed turn and
	// the release of turns 2..N.
	FollowUpStartDelay time.Duration `json:"follow_up_start_delay"`

	// ReadyWaitGroup is used to coordinate thundering-herd fanout from the CLI
	// layer. Each runner calls Done() once it is ready to start.
	ReadyWaitGroup *sync.WaitGroup `json:"-"`

	// StartChan blocks runners before creating chats. The CLI layer closes it
	// once all runners have reached the start barrier.
	StartChan chan struct{} `json:"-"`

	// FollowUpReadyWaitGroup coordinates the delayed follow-up phase. Each
	// runner signals once it has completed the first turn or knows it will never
	// reach that point.
	FollowUpReadyWaitGroup *sync.WaitGroup `json:"-"`

	// StartFollowUpChan blocks turns 2..N until the CLI layer releases the
	// follow-up phase.
	StartFollowUpChan chan struct{} `json:"-"`

	Metrics           *Metrics `json:"-"`
	MetricLabelValues []string `json:"metric_label_values"`
}

func (c Config) HasFollowUps() bool {
	return c.Turns > 1
}

func (c Config) ShouldGateFollowUps() bool {
	return c.HasFollowUps() && c.FollowUpStartDelay > 0
}

func (c Config) PromptForTurn(turnIndex int) (string, error) {
	if turnIndex < 0 || turnIndex >= c.Turns {
		return "", xerrors.Errorf("turn index %d out of range [0,%d)", turnIndex, c.Turns)
	}

	basePrompt := c.Prompt
	if turnIndex > 0 {
		basePrompt = c.FollowUpPrompt
	}
	if c.ToolCallsPerChat == 0 {
		return basePrompt, nil
	}

	toolCallsByTurn := chatcontrol.ToolCallsByTurn(c.Turns, c.ToolCallsPerChat, c.ToolCallSeed)
	toolCallsThisTurn := toolCallsByTurn[turnIndex]
	if toolCallsThisTurn == 0 {
		return basePrompt, nil
	}
	control := chatcontrol.Control{
		ToolCallsThisTurn: toolCallsThisTurn,
		Tool:              c.ToolCallTool,
		Command:           c.ToolCallCommand,
	}
	prompt, err := chatcontrol.PrefixPrompt(basePrompt, control)
	if err != nil {
		return "", xerrors.Errorf("prefix prompt for turn %d: %w", turnIndex+1, err)
	}
	return prompt, nil
}

func (c Config) UsesExistingWorkspace() bool {
	return c.WorkspaceID != uuid.Nil
}

func (c Config) CreatesWorkspace() bool {
	return c.hasWorkspaceConfig()
}

func (c Config) hasWorkspaceConfig() bool {
	return c.Workspace.OrganizationID != uuid.Nil ||
		c.Workspace.UserID != "" ||
		c.Workspace.Request.TemplateID != uuid.Nil ||
		c.Workspace.Request.TemplateVersionID != uuid.Nil ||
		c.Workspace.Request.Name != "" ||
		len(c.Workspace.Request.RichParameterValues) > 0 ||
		c.Workspace.NoWaitForAgents ||
		c.Workspace.NoWaitForBuild ||
		c.Workspace.Retry != 0
}

func (c *Config) Validate() error {
	if c.RunID == "" {
		return xerrors.Errorf("validate run_id: must not be empty")
	}

	switch {
	case c.UsesExistingWorkspace() && c.CreatesWorkspace():
		return xerrors.Errorf("validate workspace selection: exactly one of workspace_id or workspace config must be set")
	case c.UsesExistingWorkspace():
		// Shared-workspace mode is valid as-is.
	case c.CreatesWorkspace():
		if err := c.Workspace.Validate(); err != nil {
			return xerrors.Errorf("validate workspace: %w", err)
		}
	default:
		return xerrors.Errorf("validate workspace selection: exactly one of workspace_id or workspace config must be set")
	}

	if c.Prompt == "" {
		return xerrors.Errorf("validate prompt: must not be empty")
	}

	if c.Turns < 1 {
		return xerrors.Errorf("validate turns: must be at least 1")
	}

	if c.ToolCallsPerChat < 0 {
		return xerrors.Errorf("validate tool_calls_per_chat: must not be negative")
	}
	if c.ToolCallsPerChat > c.Turns*MaxToolCallStepsPerTurn {
		return xerrors.Errorf("validate tool_calls_per_chat: must be at most %d for %d turns", c.Turns*MaxToolCallStepsPerTurn, c.Turns)
	}
	if c.ToolCallsPerChat > 0 && c.ToolCallCommand == "" {
		return xerrors.Errorf("validate tool_call_command: must not be empty when tool_calls_per_chat > 0")
	}

	if c.HasFollowUps() && c.FollowUpPrompt == "" {
		return xerrors.Errorf("validate follow_up_prompt: must not be empty when turns > 1")
	}

	if c.FollowUpStartDelay < 0 {
		return xerrors.Errorf("validate follow_up_start_delay: must not be negative")
	}

	if c.ReadyWaitGroup == nil {
		return xerrors.Errorf("validate ready_wait_group: must not be nil")
	}

	if c.StartChan == nil {
		return xerrors.Errorf("validate start_chan: must not be nil")
	}

	if c.ShouldGateFollowUps() && c.FollowUpReadyWaitGroup == nil {
		return xerrors.Errorf("validate follow_up_ready_wait_group: must not be nil when follow-up delay is enabled")
	}

	if c.ShouldGateFollowUps() && c.StartFollowUpChan == nil {
		return xerrors.Errorf("validate start_follow_up_chan: must not be nil when follow-up delay is enabled")
	}

	if c.Metrics == nil {
		return xerrors.Errorf("validate metrics: must not be nil")
	}

	return nil
}
