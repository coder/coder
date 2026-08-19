package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge/annotations"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

// MCPToolNameAnnotateInterception is the tool that records work context on an
// AI Gateway interception.
const MCPToolNameAnnotateInterception = "coder_annotate_interception"

// MCPAnnotationsInstructions is the server instruction text used by the
// annotations toolset. It is sent during initialization, so a client that
// honors server instructions sees it before the first tool call.
const MCPAnnotationsInstructions = "Coder AI Gateway work-context annotation. " +
	"Every request you send through the Coder AI Gateway is recorded. Call " +
	MCPToolNameAnnotateInterception + " to attach the repository, branch, " +
	"Linear issues, and GitHub pull requests the current work belongs to, so " +
	"the recorded activity can be attributed later. Call it as soon as you " +
	"know any of those values, and again whenever they change or you learn a " +
	"new issue or pull request. When the session context supplies an AI " +
	"Gateway session ID, pass it as session_id on every call. Never ask the " +
	"user for these values and never guess them."

const mcpAnnotateInterceptionDescription = "Record the work context for the " +
	"current AI Gateway activity so it can be attributed later. Call this as " +
	"soon as you know the repository, branch, or Linear issues you are " +
	"working on, and again whenever any of them change, including when you " +
	"open a pull request. Supply only the fields you are confident about; " +
	"omitted fields keep their previous value. Issues and pull requests " +
	"accumulate, so passing a new one keeps the earlier ones. Pass session_id " +
	"whenever the session context supplies one. Do not guess."

type mcpAnnotateInterceptionArgs struct {
	LinearIssueIDs []string `json:"linear_issue_ids"`
	GitHubPRURLs   []string `json:"github_pr_urls"`
	Repo           string   `json:"repo"`
	Branch         string   `json:"branch"`
	SessionID      string   `json:"session_id"`
}

var mcpAnnotateInterceptionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"linear_issue_ids": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": `Linear issue identifiers the work belongs to, e.g. ["ENG-1234"]. These are added to the issues already recorded rather than replacing them.`,
		},
		"github_pr_urls": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": `GitHub pull request URLs the work produced, e.g. ["https://github.com/coder/coder/pull/1234"]. These are added to the pull requests already recorded rather than replacing them.`,
		},
		"repo": map[string]any{
			"type":        "string",
			"description": `Repository the work targets, e.g. "coder/coder".`,
		},
		"branch": map[string]any{
			"type":        "string",
			"description": "Git branch the work targets.",
		},
		"session_id": map[string]any{
			"type":        "string",
			"description": "AI Gateway session identifier supplied by the session context. Pass it verbatim whenever it is available so the annotation lands on this session rather than the most recent activity. Omit it when the context does not supply one, and never guess it.",
		},
	},
	"required": []string{},
}

// registerMCPAnnotationTool registers the annotation tool on srv. The tool
// annotates the interception matching the supplied session ID, or the most
// recent interception initiated by initiatorID when no session ID is given.
// Every lookup is scoped to initiatorID, so a session ID supplied by the model
// can only reach that user's own interceptions. The write runs with the actor
// installed on the request context by the API key middleware, so the caller's
// own permissions authorize it.
func registerMCPAnnotationTool(srv *mcp.Server, db database.Store, initiatorID uuid.UUID) error {
	if db == nil {
		return xerrors.New("database cannot be nil")
	}
	if initiatorID == uuid.Nil {
		return xerrors.New("initiator ID cannot be nil")
	}

	srv.AddTool(&sdkmcp.Tool{
		Name:        MCPToolNameAnnotateInterception,
		Description: mcpAnnotateInterceptionDescription,
		InputSchema: mcpAnnotateInterceptionSchema,
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: ptr.Ref(false),
			IdempotentHint:  true,
			OpenWorldHint:   ptr.Ref(false),
		},
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var args mcpAnnotateInterceptionArgs
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return mcpToolError(xerrors.Errorf("decode arguments: %w", err)), nil
			}
		}

		params, err := annotations.Params(annotations.Input{
			LinearIssueIDs: args.LinearIssueIDs,
			GitHubPRURLs:   args.GitHubPRURLs,
			Repo:           args.Repo,
			Branch:         args.Branch,
		})
		if err != nil {
			return mcpToolError(err), nil
		}

		sessionID := strings.TrimSpace(args.SessionID)
		lookup := database.GetLatestAIBridgeInterceptionIDByInitiatorParams{
			InitiatorID: initiatorID,
		}
		if sessionID != "" {
			lookup.ClientSessionID = sql.NullString{String: sessionID, Valid: true}
		}

		// The lookup runs as the system because a user may annotate an
		// interception without being allowed to read interceptions. It
		// returns an identifier only, filtered to initiatorID.
		//nolint:gocritic // See above.
		interceptionID, err := db.GetLatestAIBridgeInterceptionIDByInitiator(dbauthz.AsSystemRestricted(ctx), lookup)
		if errors.Is(err, sql.ErrNoRows) {
			if sessionID != "" {
				// A supplied session ID that matches nothing is reported
				// rather than widened to the latest interception, so a
				// wrong ID cannot annotate unrelated activity.
				return mcpToolError(xerrors.Errorf("no AI Gateway interception for session %q", sessionID)), nil
			}
			return mcpToolError(xerrors.New("no AI Gateway interception to annotate")), nil
		}
		if err != nil {
			return mcpToolError(xerrors.Errorf("load latest interception: %w", err)), nil
		}

		params.ID = interceptionID
		updated, err := db.UpdateAIBridgeInterceptionAnnotations(ctx, params)
		if err != nil {
			return mcpToolError(xerrors.Errorf("annotate interception: %w", err)), nil
		}

		// The result omits the annotations read back from the row so
		// server-derived keys such as capabilities stay out of the
		// conversation.
		result, err := json.Marshal(map[string]any{
			"annotated":       true,
			"interception_id": updated.ID.String(),
		})
		if err != nil {
			return mcpToolError(xerrors.Errorf("encode result: %w", err)), nil
		}
		return &sdkmcp.CallToolResult{
			Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(result)}},
		}, nil
	})
	return nil
}

// mcpToolError reports a failure the model can act on, rather than failing the
// JSON-RPC call.
func mcpToolError(err error) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		IsError: true,
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: err.Error()}},
	}
}
