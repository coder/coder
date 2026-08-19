// Package annotations validates client-asserted work context before it is
// written to the annotations column of an AI Bridge interception.
package annotations

import (
	"database/sql"
	"regexp"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// MaxValueLength bounds each annotation value. The values are model generated
// and are stored verbatim in a JSONB column that is also read back into CSV
// exports.
const MaxValueLength = 256

// MaxLinearIssueIDs bounds how many issues a single call may add. The stored
// set can still grow across calls.
const MaxLinearIssueIDs = 16

// MaxGitHubPRURLs bounds how many pull request URLs a single call may add. The
// stored set can still grow across calls.
const MaxGitHubPRURLs = 16

// githubPRURLPattern matches the canonical GitHub pull request URL. The stored
// value is rendered as a link target, so anything else, including other
// schemes and hosts, is rejected before it reaches the database.
var githubPRURLPattern = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+/pull/[0-9]+$`)

// Input is the untrusted work context supplied by a caller.
type Input struct {
	LinearIssueIDs []string
	GitHubPRURLs   []string
	Repo           string
	Branch         string
}

// Params trims the input and maps empty values to NULL, which the update query
// treats as "leave unchanged". It returns an error when every field is empty,
// because such a call would write nothing. The returned params carry no
// interception ID; the caller sets it.
func Params(input Input) (database.UpdateAIBridgeInterceptionAnnotationsParams, error) {
	var params database.UpdateAIBridgeInterceptionAnnotationsParams
	for _, field := range []struct {
		name  string
		value string
		dest  *sql.NullString
	}{
		{"repo", input.Repo, &params.Repo},
		{"branch", input.Branch, &params.Branch},
	} {
		value, err := trimValue(field.name, field.value)
		if err != nil {
			return database.UpdateAIBridgeInterceptionAnnotationsParams{}, err
		}
		if value == "" {
			continue
		}
		*field.dest = sql.NullString{String: value, Valid: true}
	}

	if len(input.LinearIssueIDs) > MaxLinearIssueIDs {
		return database.UpdateAIBridgeInterceptionAnnotationsParams{}, xerrors.Errorf(
			"linear_issue_ids accepts at most %d issues per call", MaxLinearIssueIDs,
		)
	}
	var issues []string
	for _, issue := range input.LinearIssueIDs {
		value, err := trimValue("linear_issue_ids", issue)
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

	if len(input.GitHubPRURLs) > MaxGitHubPRURLs {
		return database.UpdateAIBridgeInterceptionAnnotationsParams{}, xerrors.Errorf(
			"github_pr_urls accepts at most %d URLs per call", MaxGitHubPRURLs,
		)
	}
	var prs []string
	for _, pr := range input.GitHubPRURLs {
		value, err := trimValue("github_pr_urls", pr)
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

	if len(params.LinearIssueIds) == 0 && len(params.GithubPrUrls) == 0 &&
		!params.Repo.Valid && !params.Branch.Valid {
		return database.UpdateAIBridgeInterceptionAnnotationsParams{}, xerrors.New(
			"provide at least one of linear_issue_ids, github_pr_urls, repo, or branch",
		)
	}
	return params, nil
}

func trimValue(name string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > MaxValueLength {
		return "", xerrors.Errorf(
			"%s must be %d characters or fewer", name, MaxValueLength,
		)
	}
	return value, nil
}
