package chattool

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
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
	LinearIssueIDs []string `json:"linear_issue_ids,omitempty" description:"Linear issue identifiers the work belongs to, e.g. [\"ENG-1234\"]. These are added to the issues already recorded rather than replacing them."`
	GitHubPRURLs   []string `json:"github_pr_urls,omitempty" description:"GitHub pull request URLs the work produced, e.g. [\"https://github.com/coder/coder/pull/1234\"]. These are added to the pull requests already recorded rather than replacing them."`
	Repo           string   `json:"repo,omitempty" description:"Repository the work targets, e.g. \"coder/coder\"."`
	Branch         string   `json:"branch,omitempty" description:"Git branch the work targets."`
}

// maxAnnotationValueLength bounds each annotation value. The values are model
// generated and are stored verbatim in a JSONB column that is also read back
// into CSV exports.
const maxAnnotationValueLength = 256

// maxLinearIssueIDs bounds how many issues a single call may add. The stored
// set can still grow across calls.
const maxLinearIssueIDs = 16

// maxGitHubPRURLs bounds how many pull request URLs a single call may add. The
// stored set can still grow across calls.
const maxGitHubPRURLs = 16

// githubPRURLPattern matches the canonical GitHub pull request URL. The stored
// value is rendered as a link target, so anything else, including other
// schemes and hosts, is rejected before it reaches the database.
var githubPRURLPattern = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+/pull/[0-9]+$`)

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
			params, err := annotationParams(args)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if len(params.LinearIssueIds) == 0 && len(params.GithubPrUrls) == 0 &&
				!params.Repo.Valid && !params.Branch.Valid {
				return fantasy.NewTextErrorResponse(
					"provide at least one of linear_issue_ids, github_pr_urls, repo, or branch",
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
		{"repo", args.Repo, &params.Repo},
		{"branch", args.Branch, &params.Branch},
	} {
		value, err := trimAnnotationValue(field.name, field.value)
		if err != nil {
			return database.UpdateAIBridgeInterceptionAnnotationsParams{}, err
		}
		if value == "" {
			continue
		}
		*field.dest = sql.NullString{String: value, Valid: true}
	}

	if len(args.LinearIssueIDs) > maxLinearIssueIDs {
		return database.UpdateAIBridgeInterceptionAnnotationsParams{}, xerrors.Errorf(
			"linear_issue_ids accepts at most %d issues per call", maxLinearIssueIDs,
		)
	}
	var issues []string
	for _, issue := range args.LinearIssueIDs {
		value, err := trimAnnotationValue("linear_issue_ids", issue)
		if err != nil {
			return database.UpdateAIBridgeInterceptionAnnotationsParams{}, err
		}
		if value == "" {
			continue
		}
		issues = append(issues, value)
	}
	if len(issues) > 0 {
		params.LinearIssueIds = issues
	}

	if len(args.GitHubPRURLs) > maxGitHubPRURLs {
		return database.UpdateAIBridgeInterceptionAnnotationsParams{}, xerrors.Errorf(
			"github_pr_urls accepts at most %d URLs per call", maxGitHubPRURLs,
		)
	}
	var prs []string
	for _, pr := range args.GitHubPRURLs {
		value, err := trimAnnotationValue("github_pr_urls", pr)
		if err != nil {
			return database.UpdateAIBridgeInterceptionAnnotationsParams{}, err
		}
		if value == "" {
			continue
		}
		if !githubPRURLPattern.MatchString(value) {
			return database.UpdateAIBridgeInterceptionAnnotationsParams{}, xerrors.Errorf(
				"github_pr_urls entry %q must look like https://github.com/{owner}/{repo}/pull/{number}", value,
			)
		}
		prs = append(prs, value)
	}
	if len(prs) > 0 {
		params.GithubPrUrls = prs
	}
	return params, nil
}

func trimAnnotationValue(name string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > maxAnnotationValueLength {
		return "", xerrors.Errorf(
			"%s must be %d characters or fewer", name, maxAnnotationValueLength,
		)
	}
	return value, nil
}
