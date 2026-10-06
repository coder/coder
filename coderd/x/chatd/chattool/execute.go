package chattool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/toolsdk/workspacetools"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// maxOutputToModel is the maximum output sent to the LLM.
const maxOutputToModel = workspacetools.MaxOutputBytes

// toolNames are the Coder Agents names of the workspace tools, used in
// shared descriptions and follow-up hints.
var toolNames = workspacetools.ToolNames{
	Execute:       ExecuteToolName,
	ProcessOutput: "process_output",
	ProcessList:   "process_list",
	ReadFile:      "read_file",
}

// ExecuteResult is the structured response from the execute
// tool.
type ExecuteResult = workspacetools.ExecuteResult

// ExecuteOptions configures the execute tool.
type ExecuteOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	DefaultTimeout   time.Duration
	// AgentBrowserSession, when non-empty, is exported as
	// AGENT_BROWSER_SESSION so agent-browser CLI invocations land in a
	// browser session scoped to this chat instead of a shared default.
	AgentBrowserSession string
}

// ProcessToolOptions configures a process management tool
// (process_output, process_list, or process_signal). Each of
// these tools only needs a workspace connection resolver.
type ProcessToolOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
}

// ExecuteArgs are the parameters accepted by the execute tool. Keep the
// shared descriptions in sync with workspacetools; a parity test in
// coderd/mcp compares them.
type ExecuteArgs struct {
	Command         string  `json:"command" description:"The shell command to execute. Runs under \"sh -c\" (POSIX)."`
	ModelIntent     *string `json:"model_intent,omitempty" description:"A short, natural-language, present-participle phrase describing what you are doing. This is shown to the user alongside the command, with backgrounded commands framed as \"<intent> in the background using <command>\", so do not include the word \"background\" or restate the command or a duration. Use plain English with no underscores or technical jargon. Keep it under 100 characters. Good examples: \"Running the unit tests\", \"Checking repository state\", \"Inspecting build output\"."`
	Timeout         *string `json:"timeout,omitempty" description:"How long to wait for completion (e.g. '30s', '5m'). Default is 10s. The process keeps running if this expires and you get a background_process_id to re-attach. Only applies to foreground commands."`
	WorkDir         *string `json:"workdir,omitempty" description:"Working directory for the command."`
	RunInBackground *bool   `json:"run_in_background,omitempty" description:"Run without blocking. Use for persistent processes (dev servers, file watchers) or when you want to continue working while a command runs and check the result later with process_output. For commands whose result you need before continuing, prefer foreground with a longer timeout. Use this parameter instead of shell '&', which leaves an untracked process that process_output cannot read."`
}

// ExecuteToolName is the registered name of the execute tool.
const ExecuteToolName = "execute"

// Execute returns an AgentTool that runs a shell command in the
// workspace via the agent HTTP API.
func Execute(options ExecuteOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ExecuteToolName,
		workspacetools.ExecuteDescription(toolNames),
		func(ctx context.Context, args ExecuteArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return executeTool(ctx, conn, args, options), nil
		},
	)
}

func executeTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args ExecuteArgs,
	options ExecuteOptions,
) fantasy.ToolResponse {
	env := map[string]string{"CODER_CHAT_AGENT": "true"}
	if options.AgentBrowserSession != "" {
		env["AGENT_BROWSER_SESSION"] = options.AgentBrowserSession
	}
	req := workspacetools.ExecuteRequest{
		Command:         args.Command,
		Timeout:         args.Timeout,
		RunInBackground: args.RunInBackground != nil && *args.RunInBackground,
		DefaultTimeout:  options.DefaultTimeout,
		Env:             env,
		Names:           toolNames,
	}
	if args.WorkDir != nil {
		req.WorkDir = *args.WorkDir
	}
	result, err := workspacetools.Execute(ctx, conn, req)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}
	return marshalExecuteResult(result)
}

func marshalExecuteResult(result ExecuteResult) fantasy.ToolResponse {
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}
	return fantasy.NewTextResponse(string(data))
}

// exitedResult converts the output of a finished process to a result.
func exitedResult(resp workspacesdk.ProcessOutputResponse) ExecuteResult {
	return workspacetools.ExitedResult(resp)
}

// errorResult builds a ToolResponse from an ExecuteResult with
// an error message.
func errorResult(msg string) fantasy.ToolResponse {
	return marshalExecuteResult(ExecuteResult{Success: false, Error: msg})
}

// ProcessOutputArgs are the parameters accepted by the
// process_output tool.
type ProcessOutputArgs struct {
	ProcessID   string  `json:"process_id"`
	WaitTimeout *string `json:"wait_timeout,omitempty" description:"Override the default 10s block duration. The call blocks until the process exits or this timeout is reached. Set to '0s' for an immediate snapshot without waiting."`
	ModelIntent *string `json:"model_intent,omitempty" description:"A short, natural-language, present-participle phrase describing why you are checking this process. This is shown as the user's primary label for the action, so make it self-sufficient: the command itself is not displayed alongside it. Use plain English with no underscores or technical jargon. Do not restate the command or include a duration. Keep it under 100 characters. Good examples: \"Waiting for the dev server to be ready\", \"Confirming the tests still pass\"."`
}

// ProcessOutput returns an AgentTool that retrieves the output
// of a tracked process by its ID.
func ProcessOutput(options ProcessToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		toolNames.ProcessOutput,
		workspacetools.ProcessOutputDescription(toolNames),
		func(ctx context.Context, args ProcessOutputArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			if args.ProcessID == "" {
				return fantasy.NewTextErrorResponse("process_id is required"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			result, err := workspacetools.ProcessOutput(ctx, conn, workspacetools.ProcessOutputRequest{
				ProcessID:   args.ProcessID,
				WaitTimeout: args.WaitTimeout,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return marshalExecuteResult(result), nil
		},
	)
}

// ProcessList returns an AgentTool that lists all tracked
// processes on the workspace agent.
func ProcessList(options ProcessToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		toolNames.ProcessList,
		workspacetools.ProcessListDescription,
		func(ctx context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			resp, err := conn.ListProcesses(ctx)
			if err != nil {
				return errorResult(fmt.Sprintf("list processes: %v", err)), nil
			}
			data, err := json.Marshal(resp)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return fantasy.NewTextResponse(string(data)), nil
		},
	)
}

// ProcessSignalArgs are the parameters accepted by the
// process_signal tool.
type ProcessSignalArgs struct {
	ProcessID string `json:"process_id"`
	Signal    string `json:"signal"`
}

// ProcessSignal returns an AgentTool that sends a signal to a
// tracked process on the workspace agent by its ID.
func ProcessSignal(options ProcessToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"process_signal",
		workspacetools.ProcessSignalDescription(toolNames),
		func(ctx context.Context, args ProcessSignalArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			if args.ProcessID == "" {
				return fantasy.NewTextErrorResponse("process_id is required"), nil
			}
			if err := workspacetools.ValidateSignal(args.Signal); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if err := conn.SignalProcess(ctx, args.ProcessID, args.Signal); err != nil {
				return errorResult(fmt.Sprintf("signal process: %v", err)), nil
			}
			return marshalToolResponse(workspacetools.SignalSent(args.ProcessID, args.Signal)), nil
		},
	)
}
