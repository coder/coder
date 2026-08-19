package chattool

import (
	"context"
	"database/sql"
	"errors"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge/annotations"
	"github.com/coder/coder/v2/coderd/database"
)

// AnnotateInterceptionOptions configures the annotate_interception tool.
type AnnotateInterceptionOptions struct {
	// OwnerID is the user whose AI Bridge interceptions are annotated. It is
	// also the identity the database calls are authorized as.
	OwnerID uuid.UUID
}

type annotateInterceptionArgs struct {
	LinearIssueIDs []string `json:"linear_issue_ids,omitempty" description:"Linear issue identifiers the work belongs to, e.g. [\"ENG-1234\"]. These are added to the issues already recorded rather than replacing them."`
	GitHubPRURLs   []string `json:"github_pr_urls,omitempty" description:"GitHub pull request URLs the work produced, e.g. [\"https://github.com/coder/coder/pull/1234\"]. These are added to the pull requests already recorded rather than replacing them."`
	Repo           string   `json:"repo,omitempty" description:"Repository the work targets, e.g. \"coder/coder\"."`
	Branch         string   `json:"branch,omitempty" description:"Git branch the work targets."`
}

// AnnotateInterception returns a tool that records work context on the AI
// Bridge interception carrying the current model request. db must not be nil
// and options.OwnerID must not be uuid.Nil.
func AnnotateInterception(db database.Store, options AnnotateInterceptionOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"annotate_interception",
		"Record the work context for the current model request so this "+
			"activity can be attributed later. Call it as soon as you know "+
			"the repository, branch, or Linear issues you are working on, and "+
			"again whenever any of them change, including when you open a "+
			"pull request. Supply only the fields you "+
			"are confident about; omitted fields keep their previous value. "+
			"Issues and pull requests accumulate, so pass a new one as soon "+
			"as the work touches it and earlier ones are kept. Do not guess.",
		func(ctx context.Context, args annotateInterceptionArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			params, err := annotations.Params(annotations.Input{
				LinearIssueIDs: args.LinearIssueIDs,
				GitHubPRURLs:   args.GitHubPRURLs,
				Repo:           args.Repo,
				Branch:         args.Branch,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			ownerCtx, err := asOwner(ctx, db, options.OwnerID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			interception, err := db.GetLatestAIBridgeInterceptionByInitiator(ownerCtx, options.OwnerID)
			if errors.Is(err, sql.ErrNoRows) {
				return fantasy.NewTextErrorResponse(
					"no AI Bridge interception to annotate",
				), nil
			}
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("load latest interception: %w", err).Error(),
				), nil
			}

			params.ID = interception.ID
			updated, err := db.UpdateAIBridgeInterceptionAnnotations(ownerCtx, params)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("annotate interception: %w", err).Error(),
				), nil
			}

			// The response omits the annotations read back from the row so
			// server-derived keys such as capabilities stay out of the
			// conversation.
			return toolResponse(map[string]any{
				"annotated":       true,
				"interception_id": updated.ID.String(),
			}), nil
		},
	)
}
