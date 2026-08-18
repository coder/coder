package chattool

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// AnnotateInterceptionOptions configures the annotate_interception tool.
type AnnotateInterceptionOptions struct {
	// OwnerID is the user whose AI Bridge interceptions are annotated. It is
	// also the identity the database calls are authorized as.
	OwnerID uuid.UUID
}

type annotateInterceptionArgs struct {
	LinearIssueID string `json:"linear_issue_id,omitempty" description:"Linear issue identifier the work belongs to, e.g. \"ENG-1234\"."`
	Repo          string `json:"repo,omitempty" description:"Repository the work targets, e.g. \"coder/coder\"."`
	Branch        string `json:"branch,omitempty" description:"Git branch the work targets."`
}

// maxAnnotationValueLength bounds each annotation value. The values are model
// generated and are stored verbatim in a JSONB column that is also read back
// into CSV exports.
const maxAnnotationValueLength = 256

// AnnotateInterception returns a tool that records work context on the AI
// Bridge interception carrying the current model request. db must not be nil
// and options.OwnerID must not be uuid.Nil.
func AnnotateInterception(db database.Store, options AnnotateInterceptionOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"annotate_interception",
		"Record the work context for the current model request so this "+
			"activity can be attributed later. Call it as soon as you know "+
			"the repository, branch, or Linear issue you are working on, and "+
			"again whenever any of them change. Supply only the fields you "+
			"are confident about; omitted fields keep their previous value. "+
			"Do not guess.",
		func(ctx context.Context, args annotateInterceptionArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			params, err := annotationParams(args)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if !params.LinearIssueID.Valid && !params.Repo.Valid && !params.Branch.Valid {
				return fantasy.NewTextErrorResponse(
					"provide at least one of linear_issue_id, repo, or branch",
				), nil
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

// annotationParams trims the arguments and maps empty values to NULL, which
// the update query treats as "leave unchanged".
func annotationParams(args annotateInterceptionArgs) (database.UpdateAIBridgeInterceptionAnnotationsParams, error) {
	var params database.UpdateAIBridgeInterceptionAnnotationsParams
	for _, field := range []struct {
		name  string
		value string
		dest  *sql.NullString
	}{
		{"linear_issue_id", args.LinearIssueID, &params.LinearIssueID},
		{"repo", args.Repo, &params.Repo},
		{"branch", args.Branch, &params.Branch},
	} {
		value := strings.TrimSpace(field.value)
		if value == "" {
			continue
		}
		if len(value) > maxAnnotationValueLength {
			return database.UpdateAIBridgeInterceptionAnnotationsParams{}, xerrors.Errorf(
				"%s must be %d characters or fewer", field.name, maxAnnotationValueLength,
			)
		}
		*field.dest = sql.NullString{String: value, Valid: true}
	}
	return params, nil
}
