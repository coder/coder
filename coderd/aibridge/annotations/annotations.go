// Package annotations validates client-asserted work context before it is
// written to the annotations column of an AI Bridge interception.
package annotations

import (
	"regexp"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// MaxValueLength bounds each annotation value. The values are model generated
// and are stored verbatim in a JSONB column that is also read back into CSV
// exports.
const MaxValueLength = 256

// MaxValuesPerKey bounds how many values a single call may add to one key. The
// stored set can still grow across calls.
const MaxValuesPerKey = 16

// githubPRURLPattern matches the canonical GitHub pull request URL. The stored
// value is rendered as a link target, so anything else, including other
// schemes and hosts, is rejected before it reaches the database.
var githubPRURLPattern = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+/pull/[0-9]+$`)

// Input is the untrusted work context supplied by a caller. Every key holds a
// set of values, which the update query unions with the values already stored.
type Input struct {
	LinearIssueIDs []string
	GitHubPRURLs   []string
	Repos          []string
	Branches       []string
}

// Params trims the input and leaves an empty key NULL, which the update query
// treats as "leave unchanged". It returns an error when every key is empty,
// because such a call would write nothing. The returned params carry no
// interception ID; the caller sets it.
func Params(input Input) (database.UpdateAIBridgeInterceptionAnnotationsParams, error) {
	var params database.UpdateAIBridgeInterceptionAnnotationsParams
	for _, key := range []struct {
		name     string
		values   []string
		validate func(string) error
		dest     *[]string
	}{
		{"linear_issue_ids", input.LinearIssueIDs, nil, &params.LinearIssueIds},
		{"github_pr_urls", input.GitHubPRURLs, validateGitHubPRURL, &params.GithubPrUrls},
		{"repos", input.Repos, nil, &params.Repos},
		{"branches", input.Branches, nil, &params.Branches},
	} {
		values, err := cleanValues(key.name, key.values, key.validate)
		if err != nil {
			return database.UpdateAIBridgeInterceptionAnnotationsParams{}, err
		}
		*key.dest = values
	}

	if len(params.LinearIssueIds) == 0 && len(params.GithubPrUrls) == 0 &&
		len(params.Repos) == 0 && len(params.Branches) == 0 {
		return database.UpdateAIBridgeInterceptionAnnotationsParams{}, xerrors.New(
			"provide at least one of linear_issue_ids, github_pr_urls, repos, or branches",
		)
	}
	return params, nil
}

// cleanValues trims each value, drops the empty ones, and applies validate to
// what remains. It returns nil when nothing survives, so the key is left
// unchanged rather than written as an empty set.
func cleanValues(name string, values []string, validate func(string) error) ([]string, error) {
	if len(values) > MaxValuesPerKey {
		return nil, xerrors.Errorf(
			"%s accepts at most %d values per call", name, MaxValuesPerKey,
		)
	}

	var cleaned []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > MaxValueLength {
			return nil, xerrors.Errorf(
				"%s must be %d characters or fewer", name, MaxValueLength,
			)
		}
		if validate != nil {
			if err := validate(value); err != nil {
				return nil, err
			}
		}
		cleaned = append(cleaned, value)
	}
	return cleaned, nil
}

func validateGitHubPRURL(value string) error {
	if !githubPRURLPattern.MatchString(value) {
		return xerrors.Errorf(
			"github_pr_urls entry %q must look like https://github.com/{owner}/{repo}/pull/{number}", value,
		)
	}
	return nil
}
