// Package main implements the deep-review skill's CLI tool.
//
// It provides subcommands for managing structured code review
// findings, compiling reviewer output, fetching PR context from
// GitHub, building review payloads, and posting reviews.
//
// Invoked via: go run .agents/skills/deep-review/scripts <subcommand> [flags]
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// --- Severity ---

// Severity represents a finding's severity level.
type Severity string

const (
	P0  Severity = "P0"
	P1  Severity = "P1"
	P2  Severity = "P2"
	P3  Severity = "P3"
	P4  Severity = "P4"
	Obs Severity = "Obs"
	Nit Severity = "Nit"
)

var severityRank = map[Severity]int{
	P0: 0, P1: 1, P2: 2, P3: 3, P4: 4, Obs: 5, Nit: 6,
}

// Rank returns the numeric rank of a severity (lower = more severe).
func (s Severity) Rank() int {
	if r, ok := severityRank[s]; ok {
		return r
	}
	return 999
}

// ParseSeverity validates and returns a Severity from a string.
func ParseSeverity(s string) (Severity, error) {
	sev := Severity(s)
	if _, ok := severityRank[sev]; !ok {
		return "", fmt.Errorf("invalid severity %q: must be one of P0, P1, P2, P3, P4, Obs, Nit", s)
	}
	return sev, nil
}

// --- Finding ---

// Finding represents a single reviewer finding.
type Finding struct {
	Severity Severity `json:"severity"`
	File     *string  `json:"file"`
	Line     *int     `json:"line"`
	Summary  string   `json:"summary"`
	Evidence *string  `json:"evidence"`
	Reviewer string   `json:"reviewer"`
}

// --- Compiled findings ---

// CompiledFinding represents a group of findings at the same
// location, potentially from multiple reviewers.
type CompiledFinding struct {
	File        *string            `json:"file"`
	Line        *int               `json:"line"`
	Summary     string             `json:"summary"`
	Reviewers   []ReviewerFinding  `json:"reviewers"`
	MaxSeverity Severity           `json:"max_severity"`
	Convergent  bool               `json:"convergent"`
}

// ReviewerFinding is one reviewer's contribution to a compiled
// finding.
type ReviewerFinding struct {
	Role     string   `json:"role"`
	Severity Severity `json:"severity"`
	Summary  string   `json:"summary"`
	Evidence *string  `json:"evidence"`
}

// CompiledOutput is the top-level output of compile-findings.
type CompiledOutput struct {
	Findings []CompiledFinding `json:"findings"`
	Stats    CompileStats      `json:"stats"`
}

// CompileStats contains aggregate statistics about findings.
type CompileStats struct {
	TotalFindings      int            `json:"total_findings"`
	BySeverity         map[string]int `json:"by_severity"`
	ConvergentCount    int            `json:"convergent_count"`
	ReviewersReporting []string       `json:"reviewers_reporting"`
}

// --- Review ---

// Review is the input format for post-review.
type Review struct {
	Event            string          `json:"event"`
	Body             string          `json:"body"`
	Comments         []ReviewComment `json:"comments"`
	Replies          []ReviewReply   `json:"replies,omitempty"`
	ResolveThreadIDs []string        `json:"resolve_thread_ids,omitempty"`
}

// ReviewComment is an inline comment in a review.
type ReviewComment struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Body string `json:"body"`
}

// ReviewReply is a reply to an existing review thread.
type ReviewReply struct {
	InReplyToID int    `json:"in_reply_to_id"`
	Body        string `json:"body"`
}

// --- Main ---

func main() {
	if len(os.Args) < 2 {
		fatalf("Usage: review-tool <subcommand> [flags]\nSubcommands: add-finding, compile-findings, fetch-context, post-review, build-review")
	}
	subcmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch subcmd {
	case "add-finding":
		err = cmdAddFinding(args)
	case "compile-findings":
		err = cmdCompileFindings(args)
	case "fetch-context":
		err = cmdFetchContext(args)
	case "post-review":
		err = cmdPostReview(args)
	case "build-review":
		err = cmdBuildReview(args)
	default:
		fatalf("Unknown subcommand: %s", subcmd)
	}
	if err != nil {
		fatalf("%s: %v", subcmd, err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// --- add-finding ---

func cmdAddFinding(args []string) error {
	var output, severity, file, line, summary, evidence, reviewer string
	parseFlags(args, map[string]*string{
		"output":   &output,
		"severity": &severity,
		"file":     &file,
		"line":     &line,
		"summary":  &summary,
		"evidence": &evidence,
		"reviewer": &reviewer,
	})

	if output == "" {
		return fmt.Errorf("--output is required")
	}
	if reviewer == "" {
		return fmt.Errorf("--reviewer is required")
	}
	if summary == "" {
		return fmt.Errorf("--summary is required")
	}

	sev, err := ParseSeverity(severity)
	if err != nil {
		return err
	}

	// Validate required fields based on severity.
	isPLevel := sev.Rank() <= P4.Rank()
	if isPLevel {
		if file == "" {
			return fmt.Errorf("--file is required for %s findings", sev)
		}
		if line == "" {
			return fmt.Errorf("--line is required for %s findings", sev)
		}
		if evidence == "" {
			return fmt.Errorf("--evidence is required for %s findings", sev)
		}
	}
	if sev == Nit && file == "" {
		return fmt.Errorf("--file is required for Nit findings")
	}

	// Validate and parse line number.
	var linePtr *int
	if line != "" {
		n, err := strconv.Atoi(line)
		if err != nil || n <= 0 {
			return fmt.Errorf("--line must be a positive integer, got %q", line)
		}
		linePtr = &n

		// Warn if line exceeds file length (when file exists).
		if file != "" {
			if count, ferr := countFileLines(file); ferr == nil && n > count {
				fmt.Fprintf(os.Stderr, "Warning: --line %d exceeds %s length (%d lines)\n", n, file, count)
			}
		}
	}

	var filePtr *string
	if file != "" {
		filePtr = &file
	}
	var evidencePtr *string
	if evidence != "" {
		evidencePtr = &evidence
	}

	finding := Finding{
		Severity: sev,
		File:     filePtr,
		Line:     linePtr,
		Summary:  summary,
		Evidence: evidencePtr,
		Reviewer: reviewer,
	}

	// Read existing file or start with empty array.
	var findings []Finding
	data, err := os.ReadFile(output)
	if err == nil {
		if err := json.Unmarshal(data, &findings); err != nil {
			return fmt.Errorf("failed to parse %s: %w", output, err)
		}
	}

	findings = append(findings, finding)
	return writeJSONAtomic(output, findings)
}

// countFileLines returns the number of lines in a file.
func countFileLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

// --- compile-findings ---

func cmdCompileFindings(args []string) error {
	var dir, output string
	parseFlags(args, map[string]*string{
		"dir":    &dir,
		"output": &output,
	})

	if dir == "" {
		return fmt.Errorf("--dir is required")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading dir: %w", err)
	}

	var allFindings []Finding
	reviewerSet := map[string]bool{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not read %s: %v\n", path, err)
			continue
		}
		data = bytes.TrimSpace(data)
		if len(data) == 0 || data[0] != '[' {
			fmt.Fprintf(os.Stderr, "Skipping non-array JSON file: %s\n", path)
			continue
		}
		var findings []Finding
		if err := json.Unmarshal(data, &findings); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not parse %s: %v\n", path, err)
			continue
		}
		for _, f := range findings {
			allFindings = append(allFindings, f)
			reviewerSet[f.Reviewer] = true
		}
	}

	// Group by {file, line}.
	type groupKey struct {
		file string
		line int
	}
	groups := map[groupKey][]Finding{}
	var noLocationFindings []Finding

	for _, f := range allFindings {
		if f.File == nil || f.Line == nil {
			noLocationFindings = append(noLocationFindings, f)
			continue
		}
		key := groupKey{file: *f.File, line: *f.Line}
		groups[key] = append(groups[key], f)
	}

	var compiled []CompiledFinding

	// Process grouped findings.
	for key, findings := range groups {
		cf := buildCompiledFinding(findings)
		file := key.file
		line := key.line
		cf.File = &file
		cf.Line = &line
		compiled = append(compiled, cf)
	}

	// Process no-location findings individually.
	for _, f := range noLocationFindings {
		cf := buildCompiledFinding([]Finding{f})
		compiled = append(compiled, cf)
	}

	// Sort by severity (most severe first), then by file+line.
	sort.Slice(compiled, func(i, j int) bool {
		ri := compiled[i].MaxSeverity.Rank()
		rj := compiled[j].MaxSeverity.Rank()
		if ri != rj {
			return ri < rj
		}
		fi, fj := "", ""
		if compiled[i].File != nil {
			fi = *compiled[i].File
		}
		if compiled[j].File != nil {
			fj = *compiled[j].File
		}
		if fi != fj {
			return fi < fj
		}
		li, lj := 0, 0
		if compiled[i].Line != nil {
			li = *compiled[i].Line
		}
		if compiled[j].Line != nil {
			lj = *compiled[j].Line
		}
		return li < lj
	})

	// Build stats.
	bySeverity := map[string]int{
		"P0": 0, "P1": 0, "P2": 0, "P3": 0, "P4": 0, "Obs": 0, "Nit": 0,
	}
	convergentCount := 0
	for _, cf := range compiled {
		bySeverity[string(cf.MaxSeverity)]++
		if cf.Convergent {
			convergentCount++
		}
	}
	var reviewers []string
	for r := range reviewerSet {
		reviewers = append(reviewers, r)
	}
	sort.Strings(reviewers)

	out := CompiledOutput{
		Findings: compiled,
		Stats: CompileStats{
			TotalFindings:      len(compiled),
			BySeverity:         bySeverity,
			ConvergentCount:    convergentCount,
			ReviewersReporting: reviewers,
		},
	}

	return writeOutput(output, out)
}

func buildCompiledFinding(findings []Finding) CompiledFinding {
	var reviewers []ReviewerFinding
	reviewerNames := map[string]bool{}
	maxSev := Nit
	bestSummary := ""

	for _, f := range findings {
		reviewers = append(reviewers, ReviewerFinding{
			Role:     f.Reviewer,
			Severity: f.Severity,
			Summary:  f.Summary,
			Evidence: f.Evidence,
		})
		reviewerNames[f.Reviewer] = true
		if f.Severity.Rank() < maxSev.Rank() {
			maxSev = f.Severity
			bestSummary = f.Summary
		}
	}

	return CompiledFinding{
		Summary:     bestSummary,
		Reviewers:   reviewers,
		MaxSeverity: maxSev,
		Convergent:  len(reviewerNames) >= 2,
	}
}

// --- fetch-context ---

func cmdFetchContext(args []string) error {
	var pr, repo, output string
	dryRun := false
	parseFlagsWithBool(args, map[string]*string{
		"pr":     &pr,
		"repo":   &repo,
		"output": &output,
	}, map[string]*bool{
		"dry-run": &dryRun,
	})

	if pr == "" {
		return fmt.Errorf("--pr is required")
	}

	// Infer repo if not provided.
	if repo == "" && !dryRun {
		out, err := runGh("repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
		if err != nil {
			return fmt.Errorf("inferring repo: %w", err)
		}
		repo = strings.TrimSpace(out)
	}
	if repo == "" {
		repo = "OWNER/REPO"
	}

	owner, repoName := splitRepo(repo)

	fetches := []struct {
		key  string
		args []string
	}{
		{"pr", []string{"pr", "view", pr, "--repo", repo, "--json",
			"number,title,body,author,state,baseRefName,headRefName,url,headRefOid,baseRefOid"}},
		{"reviews", []string{"pr", "view", pr, "--repo", repo, "--json", "reviews"}},
		{"comments", []string{"pr", "view", pr, "--repo", repo, "--json", "comments"}},
		{"commits", []string{"pr", "view", pr, "--repo", repo, "--json", "commits"}},
		{"review_comments", []string{"api", "--paginate",
			fmt.Sprintf("repos/%s/pulls/%s/comments", repo, pr)}},
	}

	if dryRun {
		for _, f := range fetches {
			fmt.Printf("gh %s\n", strings.Join(f.args, " "))
		}
		fmt.Printf("gh api graphql -f query='<reviewThreads query for %s/%s#%s>'\n", owner, repoName, pr)
		return nil
	}

	// Run fetches in parallel.
	results := make(map[string]json.RawMessage)
	var mu sync.Mutex
	eg := errgroup.Group{}

	for _, f := range fetches {
		eg.Go(func() error {
			out, err := runGh(f.args...)
			if err != nil {
				return fmt.Errorf("fetching %s: %w", f.key, err)
			}
			mu.Lock()
			results[f.key] = json.RawMessage(out)
			mu.Unlock()
			return nil
		})
	}

	// Fetch review threads via GraphQL (with pagination).
	eg.Go(func() error {
		threads, err := fetchReviewThreads(owner, repoName, pr)
		if err != nil {
			return fmt.Errorf("fetching review threads: %w", err)
		}
		mu.Lock()
		results["threads"] = threads
		mu.Unlock()
		return nil
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	// Filter resolved threads from review comments.
	reviewComments, err := filterResolvedThreads(
		results["review_comments"],
		results["threads"],
	)
	if err != nil {
		return fmt.Errorf("filtering threads: %w", err)
	}

	// Assemble output.
	assembled := map[string]json.RawMessage{
		"pr":              results["pr"],
		"reviews":         extractField(results["reviews"], "reviews"),
		"review_comments": reviewComments,
		"issue_comments":  extractField(results["comments"], "comments"),
		"commits":         extractField(results["commits"], "commits"),
	}

	return writeOutput(output, assembled)
}

func splitRepo(repo string) (string, string) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return repo, ""
	}
	return parts[0], parts[1]
}

func fetchReviewThreads(owner, repoName, pr string) (json.RawMessage, error) {
	var allThreads []json.RawMessage
	cursor := ""

	for {
		afterClause := ""
		if cursor != "" {
			afterClause = fmt.Sprintf(`, after: "%s"`, cursor)
		}

		query := fmt.Sprintf(`query {
			repository(owner: "%s", name: "%s") {
				pullRequest(number: %s) {
					reviewThreads(first: 100%s) {
						pageInfo { hasNextPage endCursor }
						nodes {
							id
							isResolved
							comments(first: 1) {
								nodes { databaseId }
							}
						}
					}
				}
			}
		}`, owner, repoName, pr, afterClause)

		out, err := runGh("api", "graphql", "-f", "query="+query)
		if err != nil {
			return nil, err
		}

		var result struct {
			Data struct {
				Repository struct {
					PullRequest struct {
						ReviewThreads struct {
							PageInfo struct {
								HasNextPage bool   `json:"hasNextPage"`
								EndCursor   string `json:"endCursor"`
							} `json:"pageInfo"`
							Nodes []json.RawMessage `json:"nodes"`
						} `json:"reviewThreads"`
					} `json:"pullRequest"`
				} `json:"repository"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			return nil, fmt.Errorf("parsing GraphQL response: %w", err)
		}

		threads := result.Data.Repository.PullRequest.ReviewThreads
		allThreads = append(allThreads, threads.Nodes...)

		if !threads.PageInfo.HasNextPage {
			break
		}
		cursor = threads.PageInfo.EndCursor
	}

	return json.Marshal(allThreads)
}

// filterResolvedThreads removes review comments belonging to
// resolved threads and adds thread_id to surviving comments.
func filterResolvedThreads(commentsRaw, threadsRaw json.RawMessage) (json.RawMessage, error) {
	if commentsRaw == nil {
		return json.RawMessage("[]"), nil
	}

	// Parse threads to build resolved set and thread ID map.
	var threads []struct {
		ID         string `json:"id"`
		IsResolved bool   `json:"isResolved"`
		Comments   struct {
			Nodes []struct {
				DatabaseID int `json:"databaseId"`
			} `json:"nodes"`
		} `json:"comments"`
	}
	if threadsRaw != nil {
		if err := json.Unmarshal(threadsRaw, &threads); err != nil {
			return nil, fmt.Errorf("parsing threads: %w", err)
		}
	}

	resolvedRoots := map[int]bool{}
	threadIDMap := map[int]string{} // root comment DB ID → thread node ID
	for _, t := range threads {
		if len(t.Comments.Nodes) == 0 {
			continue
		}
		rootID := t.Comments.Nodes[0].DatabaseID
		threadIDMap[rootID] = t.ID
		if t.IsResolved {
			resolvedRoots[rootID] = true
		}
	}

	// Parse comments — we need to access individual fields but
	// pass through the rest unchanged.
	var comments []map[string]interface{}
	if err := json.Unmarshal(commentsRaw, &comments); err != nil {
		return nil, fmt.Errorf("parsing comments: %w", err)
	}

	var filtered []map[string]interface{}
	for _, c := range comments {
		// Determine root comment ID.
		commentID := jsonInt(c, "id")
		inReplyTo := jsonInt(c, "in_reply_to_id")

		rootID := commentID
		if inReplyTo != 0 {
			rootID = inReplyTo
		}

		// Skip if in resolved thread.
		if resolvedRoots[rootID] {
			continue
		}

		// Add thread_id.
		if tid, ok := threadIDMap[rootID]; ok {
			c["thread_id"] = tid
		}

		filtered = append(filtered, c)
	}

	if filtered == nil {
		filtered = []map[string]interface{}{}
	}
	return json.Marshal(filtered)
}

// --- post-review ---

func cmdPostReview(args []string) error {
	var input, pr, repo string
	dryRun := false
	parseFlagsWithBool(args, map[string]*string{
		"input": &input,
		"pr":    &pr,
		"repo":  &repo,
	}, map[string]*bool{
		"dry-run": &dryRun,
	})

	if input == "" {
		return fmt.Errorf("--input is required")
	}
	if pr == "" {
		return fmt.Errorf("--pr is required")
	}

	// Infer repo if not provided.
	if repo == "" && !dryRun {
		out, err := runGh("repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
		if err != nil {
			return fmt.Errorf("inferring repo: %w", err)
		}
		repo = strings.TrimSpace(out)
	}
	if repo == "" {
		repo = "OWNER/REPO"
	}

	data, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	var review Review
	if err := json.Unmarshal(data, &review); err != nil {
		return fmt.Errorf("parsing review JSON: %w", err)
	}

	if review.Event != "COMMENT" {
		return fmt.Errorf("event must be COMMENT, got %q", review.Event)
	}
	if review.Body == "" {
		return fmt.Errorf("body is required")
	}

	// Build the review API payload with line + side.
	type apiComment struct {
		Path        string `json:"path"`
		Line        int    `json:"line,omitempty"`
		Side        string `json:"side,omitempty"`
		SubjectType string `json:"subject_type,omitempty"`
		Body        string `json:"body"`
	}
	type apiPayload struct {
		Event    string       `json:"event"`
		Body     string       `json:"body"`
		Comments []apiComment `json:"comments,omitempty"`
	}

	payload := apiPayload{
		Event: review.Event,
		Body:  review.Body,
	}

	for _, c := range review.Comments {
		ac := apiComment{
			Path: c.Path,
			Body: c.Body,
		}
		if c.Line > 0 {
			ac.Line = c.Line
			ac.Side = "RIGHT"
		} else {
			ac.SubjectType = "file"
		}
		payload.Comments = append(payload.Comments, ac)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	endpoint := fmt.Sprintf("repos/%s/pulls/%s/reviews", repo, pr)

	if dryRun {
		fmt.Printf("POST %s\n", endpoint)
		fmt.Println(prettyJSON(payloadJSON))
		for _, r := range review.Replies {
			fmt.Printf("\nREPLY to %d\n%s\n", r.InReplyToID, r.Body)
		}
		for _, tid := range review.ResolveThreadIDs {
			fmt.Printf("\nRESOLVE thread %s\n", tid)
		}
		return nil
	}

	// Post the review.
	tmpFile, err := writeTempJSON(payloadJSON)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile)

	if _, err := runGh("api", "-X", "POST", endpoint, "--input", tmpFile); err != nil {
		return fmt.Errorf("posting review: %w", err)
	}

	// Post replies and resolve threads in parallel.
	eg := errgroup.Group{}

	for _, r := range review.Replies {
		eg.Go(func() error {
			_, err := runGh("api", "-X", "POST",
				fmt.Sprintf("repos/%s/pulls/%s/comments", repo, pr),
				"-f", fmt.Sprintf("body=%s", r.Body),
				"-F", fmt.Sprintf("in_reply_to=%d", r.InReplyToID),
			)
			if err != nil {
				return fmt.Errorf("posting reply to %d: %w", r.InReplyToID, err)
			}
			return nil
		})
	}

	for _, tid := range review.ResolveThreadIDs {
		eg.Go(func() error {
			query := fmt.Sprintf(`mutation { resolveReviewThread(input: {threadId: "%s"}) { thread { isResolved } } }`, tid)
			_, err := runGh("api", "graphql", "-f", "query="+query)
			if err != nil {
				return fmt.Errorf("resolving thread %s: %w", tid, err)
			}
			return nil
		})
	}

	return eg.Wait()
}

// --- build-review ---

func cmdBuildReview(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: build-review <init|comment|reply|resolve> [flags]")
	}
	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "init":
		return buildReviewInit(subArgs)
	case "comment":
		return buildReviewComment(subArgs)
	case "reply":
		return buildReviewReply(subArgs)
	case "resolve":
		return buildReviewResolve(subArgs)
	default:
		return fmt.Errorf("unknown build-review subcommand: %s", subcmd)
	}
}

func buildReviewInit(args []string) error {
	var output, body, event string
	parseFlags(args, map[string]*string{
		"output": &output,
		"body":   &body,
		"event":  &event,
	})

	if output == "" {
		return fmt.Errorf("--output is required")
	}
	if body == "" {
		return fmt.Errorf("--body is required")
	}
	if event == "" {
		event = "COMMENT"
	}

	// Error if file already exists.
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("file already exists: %s (delete it first or use a new path)", output)
	}

	review := Review{
		Event:            event,
		Body:             body,
		Comments:         []ReviewComment{},
		Replies:          []ReviewReply{},
		ResolveThreadIDs: []string{},
	}

	return writeJSONAtomic(output, review)
}

// modifyReview reads a review file, applies a mutation, and writes
// it back atomically.
func modifyReview(path string, fn func(*Review)) error {
	review, err := readReview(path)
	if err != nil {
		return err
	}
	fn(review)
	return writeJSONAtomic(path, review)
}

func buildReviewComment(args []string) error {
	var output, path, line, body string
	parseFlags(args, map[string]*string{
		"output": &output,
		"path":   &path,
		"line":   &line,
		"body":   &body,
	})

	if output == "" {
		return fmt.Errorf("--output is required")
	}
	if path == "" {
		return fmt.Errorf("--path is required")
	}
	if body == "" {
		return fmt.Errorf("--body is required")
	}
	if line == "" {
		return fmt.Errorf("--line is required")
	}
	lineNum, err := strconv.Atoi(line)
	if err != nil || lineNum <= 0 {
		return fmt.Errorf("--line must be a positive integer, got %q", line)
	}

	return modifyReview(output, func(r *Review) {
		r.Comments = append(r.Comments, ReviewComment{
			Path: path,
			Line: lineNum,
			Body: body,
		})
	})
}

func buildReviewReply(args []string) error {
	var output, inReplyTo, body string
	parseFlags(args, map[string]*string{
		"output":      &output,
		"in-reply-to": &inReplyTo,
		"body":        &body,
	})

	if output == "" {
		return fmt.Errorf("--output is required")
	}
	if inReplyTo == "" {
		return fmt.Errorf("--in-reply-to is required")
	}
	if body == "" {
		return fmt.Errorf("--body is required")
	}
	id, err := strconv.Atoi(inReplyTo)
	if err != nil || id <= 0 {
		return fmt.Errorf("--in-reply-to must be a positive integer, got %q", inReplyTo)
	}

	return modifyReview(output, func(r *Review) {
		r.Replies = append(r.Replies, ReviewReply{
			InReplyToID: id,
			Body:        body,
		})
	})
}

func buildReviewResolve(args []string) error {
	var output, threadID string
	parseFlags(args, map[string]*string{
		"output":    &output,
		"thread-id": &threadID,
	})

	if output == "" {
		return fmt.Errorf("--output is required")
	}
	if threadID == "" {
		return fmt.Errorf("--thread-id is required")
	}

	return modifyReview(output, func(r *Review) {
		r.ResolveThreadIDs = append(r.ResolveThreadIDs, threadID)
	})
}

func readReview(path string) (*Review, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading review file: %w (did you run 'build-review init' first?)", err)
	}
	var review Review
	if err := json.Unmarshal(data, &review); err != nil {
		return nil, fmt.Errorf("parsing review file: %w", err)
	}
	return &review, nil
}

// --- helpers ---

// parseFlags is a simple flag parser for --key value pairs.
func parseFlags(args []string, flags map[string]*string) {
	parseFlagsWithBool(args, flags, nil)
}

// parseFlagsWithBool parses --key value and --key (bool) flags.
func parseFlagsWithBool(args []string, flags map[string]*string, boolFlags map[string]*bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		name := strings.TrimPrefix(arg, "--")

		// Check bool flags first.
		if boolFlags != nil {
			if ptr, ok := boolFlags[name]; ok {
				*ptr = true
				continue
			}
		}

		// String flags need a value.
		if ptr, ok := flags[name]; ok {
			if i+1 < len(args) {
				i++
				*ptr = args[i]
			}
		}
	}
}

// runGh executes a gh CLI command and returns stdout.
func runGh(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// writeJSONAtomic writes JSON to a file atomically via tmp+rename.
func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// writeOutput writes JSON to a file or stdout.
func writeOutput(path string, v any) error {
	if path == "" {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		data = append(data, '\n')
		_, err = os.Stdout.Write(data)
		return err
	}
	return writeJSONAtomic(path, v)
}

// writeTempJSON writes JSON data to a temp file and returns the path.
func writeTempJSON(data []byte) (string, error) {
	f, err := os.CreateTemp("", "review-*.json")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

// extractField extracts a top-level field from a JSON object
// as raw JSON. Returns "[]" if the field doesn't exist.
func extractField(raw json.RawMessage, field string) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return json.RawMessage("[]")
	}
	if v, ok := m[field]; ok {
		return v
	}
	return json.RawMessage("[]")
}

// prettyJSON formats JSON for display.
func prettyJSON(data []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return string(data)
	}
	return buf.String()
}

// jsonInt extracts an int field from a map, returning 0 if missing.
func jsonInt(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}


