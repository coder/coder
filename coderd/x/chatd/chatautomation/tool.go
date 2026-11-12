package chatautomation

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	// ToolName is the name of the tool that manages chat automations.
	ToolName = "manage_automations"

	toolActionList = "list"
	toolActionGet  = "get"

	// listMaxItems and listPromptMaxRunes bound a list result so that one
	// call cannot fill the context window: at most 20 items of about 2 KB each.
	listMaxItems       = 20
	listPromptMaxRunes = 200
)

// ToolOptions configures the tool. The tool acts as OwnerID, the owner of the
// chat that holds it.
type ToolOptions struct {
	DB             database.Store
	OwnerID        uuid.UUID
	OrganizationID uuid.UUID
	Logger         slog.Logger
	Clock          quartz.Clock
}

type toolArgs struct {
	Action string `json:"action" description:"The action to run: list or get."`
	ID     string `json:"id,omitempty" description:"The automation ID. Required for get."`
}

// listItem is an automation of a list result. The prompt of an item can be cut.
type listItem struct {
	codersdk.ChatAutomation
	PromptTruncated bool `json:"prompt_truncated,omitempty"`
}

type listResult struct {
	Automations []listItem `json:"automations"`
	// Total is the number of automations that the chat owner has in the
	// organization of the chat.
	Total int    `json:"total"`
	Note  string `json:"note,omitempty"`
}

// Tool returns the manage_automations tool. It runs every call with the
// permissions of the chat owner, loaded on that call, and has no permission
// rules of its own. db must not be nil.
func Tool(options ToolOptions) fantasy.AgentTool {
	if options.Clock == nil {
		options.Clock = quartz.NewReal()
	}
	return fantasy.NewAgentTool(
		ToolName,
		"List the chat automations that you own in this organization, or get one automation by ID. "+
			"An automation sends a saved prompt to a chat from a webhook or a schedule. "+
			"A list result is cut to the newest "+strconv.Itoa(listMaxItems)+" automations and each prompt to "+
			strconv.Itoa(listPromptMaxRunes)+" characters; use get for a full prompt.",
		func(ctx context.Context, args toolArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			actor, _, err := httpmw.UserRBACSubject(ctx, options.DB, options.OwnerID, rbac.ScopeAll)
			if err != nil {
				options.Logger.Error(ctx, "load the chat owner for manage_automations", slog.Error(err))
				return fantasy.NewTextErrorResponse("the automations are unavailable"), nil
			}
			ctx = dbauthz.As(ctx, actor)

			var result any
			switch args.Action {
			case toolActionList:
				result, err = listAutomations(ctx, options)
			case toolActionGet:
				result, err = getAutomation(ctx, options, args.ID)
			default:
				return fantasy.NewTextErrorResponse(xerrors.Errorf(
					"unknown action %q: the actions are %s and %s", args.Action, toolActionList, toolActionGet).Error()), nil
			}
			if errNotFound(err) {
				return fantasy.NewTextErrorResponse("automation not found"), nil
			}
			if err != nil {
				options.Logger.Error(ctx, "manage_automations failed", slog.F("action", args.Action), slog.Error(err))
				return fantasy.NewTextErrorResponse("the automations are unavailable"), nil
			}
			data, err := json.Marshal(result)
			if err != nil {
				return fantasy.NewTextErrorResponse("the automations are unavailable"), nil
			}
			return fantasy.NewTextResponse(string(data)), nil
		},
	)
}

var errAutomationNotFound = xerrors.New("automation not found")

func errNotFound(err error) bool {
	return xerrors.Is(err, errAutomationNotFound)
}

func listAutomations(ctx context.Context, options ToolOptions) (listResult, error) {
	// The owner filter is the chat owner, also in an administrator's chat.
	rows, err := options.DB.GetChatAutomationsByOrganizationID(ctx, database.GetChatAutomationsByOrganizationIDParams{
		OrganizationID: options.OrganizationID,
		OwnerID:        options.OwnerID,
	})
	if err != nil {
		return listResult{}, xerrors.Errorf("list automations: %w", err)
	}
	total := len(rows)
	rows = rows[:min(total, listMaxItems)]
	automations := make([]database.ChatAutomation, len(rows))
	for i, r := range rows {
		automations[i] = db2sdk.ChatAutomationFromListRow(r)
	}
	refs, err := LoadRefs(ctx, options.DB, automations)
	if err != nil {
		return listResult{}, err
	}
	now := options.Clock.Now()
	result := listResult{Automations: make([]listItem, len(automations)), Total: total}
	var notes []string
	if total > len(automations) {
		notes = append(notes, "Showing the newest "+strconv.Itoa(len(automations))+" of "+strconv.Itoa(total)+" automations.")
	}
	promptCut := false
	for i, a := range automations {
		item := listItem{ChatAutomation: db2sdk.ChatAutomation(a, refs, now)}
		if utf8.RuneCountInString(item.Prompt) > listPromptMaxRunes {
			item.Prompt = string([]rune(item.Prompt)[:listPromptMaxRunes])
			item.PromptTruncated = true
			promptCut = true
		}
		result.Automations[i] = item
	}
	if promptCut {
		notes = append(notes, "A prompt with prompt_truncated is cut; use get with the id for the full prompt.")
	}
	result.Note = strings.Join(notes, " ")
	return result, nil
}

func getAutomation(ctx context.Context, options ToolOptions, rawID string) (codersdk.ChatAutomation, error) {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return codersdk.ChatAutomation{}, errAutomationNotFound
	}
	// This row holds the decrypted webhook secret. The converter never copies
	// it.
	automation, err := options.DB.GetChatAutomationByID(ctx, id)
	if httpapi.Is404Error(err) {
		return codersdk.ChatAutomation{}, errAutomationNotFound
	}
	if err != nil {
		return codersdk.ChatAutomation{}, xerrors.Errorf("get automation: %w", err)
	}
	refs, err := LoadRefs(ctx, options.DB, []database.ChatAutomation{automation})
	if err != nil {
		return codersdk.ChatAutomation{}, err
	}
	return db2sdk.ChatAutomation(automation, refs, options.Clock.Now()), nil
}
