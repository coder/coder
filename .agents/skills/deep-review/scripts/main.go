package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/coder/serpent"
)

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

var severityOrder = []Severity{P0, P1, P2, P3, P4, Obs, Nit}

func (s Severity) Rank() int {
	for i, v := range severityOrder {
		if v == s {
			return i
		}
	}
	return 999
}

func severityChoices() []string {
	out := make([]string, len(severityOrder))
	for i, s := range severityOrder {
		out[i] = string(s)
	}
	return out
}

func newSeverityBuckets() map[string]int {
	m := make(map[string]int, len(severityOrder))
	for _, s := range severityOrder {
		m[string(s)] = 0
	}
	return m
}

type Finding struct {
	Severity Severity `json:"severity"`
	File     *string  `json:"file"`
	Line     *int     `json:"line"`
	Summary  string   `json:"summary"`
	Evidence *string  `json:"evidence"`
	Reviewer string   `json:"reviewer"`
}

type CompiledFinding struct {
	File        *string           `json:"file"`
	Line        *int              `json:"line"`
	Summary     string            `json:"summary"`
	Reviewers   []ReviewerFinding `json:"reviewers"`
	MaxSeverity Severity          `json:"max_severity"`
	Convergent  bool              `json:"convergent"`
}

type ReviewerFinding struct {
	Role     string   `json:"role"`
	Severity Severity `json:"severity"`
	Summary  string   `json:"summary"`
	Evidence *string  `json:"evidence"`
}

type CompiledOutput struct {
	Findings []CompiledFinding `json:"findings"`
	Stats    CompileStats      `json:"stats"`
}

type CompileStats struct {
	TotalFindings      int            `json:"total_findings"`
	BySeverity         map[string]int `json:"by_severity"`
	ConvergentCount    int            `json:"convergent_count"`
	ReviewersReporting []string       `json:"reviewers_reporting"`
}

type Review struct {
	Event            string          `json:"event"`
	Body             string          `json:"body"`
	Comments         []ReviewComment `json:"comments"`
	Replies          []ReviewReply   `json:"replies,omitempty"`
	ResolveThreadIDs []string        `json:"resolve_thread_ids,omitempty"`
}

type ReviewComment struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Body string `json:"body"`
}

type ReviewReply struct {
	InReplyToID int    `json:"in_reply_to_id"`
	Body        string `json:"body"`
}

func main() {
	err := rootCmd().Invoke(os.Args[1:]...).WithOS().Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func rootCmd() *serpent.Command {
	return &serpent.Command{
		Use:   "review-tool",
		Short: "Deep-review skill CLI tool.",
		Children: []*serpent.Command{
			cmdAddFinding(),
			cmdCompileFindings(),
			cmdFetchContext(),
			cmdPostReview(),
			cmdBuildReview(),
		},
	}
}

func cmdAddFinding() *serpent.Command {
	var output, severity, file, line, summary, evidence, reviewer string
	return &serpent.Command{
		Use:   "add-finding",
		Short: "Append a finding to a JSON findings file.",
		Options: serpent.OptionSet{
			{
				Name:        "output",
				Description: "Path to the output JSON file.",
				Flag:        "output",
				Required:    true,
				Value:       serpent.StringOf(&output),
			},
			{
				Name:        "severity",
				Description: "Severity level of the finding.",
				Flag:        "severity",
				Required:    true,
				Value:       serpent.EnumOf(&severity, severityChoices()...),
			},
			{
				Name:        "file",
				Description: "Source file the finding applies to.",
				Flag:        "file",
				Value:       serpent.StringOf(&file),
			},
			{
				Name:        "line",
				Description: "Line number in the source file.",
				Flag:        "line",
				Value:       serpent.StringOf(&line),
			},
			{
				Name:        "summary",
				Description: "Short summary of the finding.",
				Flag:        "summary",
				Required:    true,
				Value:       serpent.StringOf(&summary),
			},
			{
				Name:        "evidence",
				Description: "Supporting evidence for the finding.",
				Flag:        "evidence",
				Value:       serpent.StringOf(&evidence),
			},
			{
				Name:        "reviewer",
				Description: "Name of the reviewer.",
				Flag:        "reviewer",
				Required:    true,
				Value:       serpent.StringOf(&reviewer),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			sev := Severity(severity)

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

			var linePtr *int
			if line != "" {
				n, err := strconv.Atoi(line)
				if err != nil || n <= 0 {
					return fmt.Errorf("--line must be a positive integer, got %q", line)
				}
				linePtr = &n

				if file != "" {
					if count, ferr := countFileLines(file); ferr == nil && n > count {
						fmt.Fprintf(inv.Stderr, "Warning: --line %d exceeds %s length (%d lines)\n", n, file, count)
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

			var findings []Finding
			data, err := os.ReadFile(output)
			if err == nil {
				if err := json.Unmarshal(data, &findings); err != nil {
					return fmt.Errorf("failed to parse %s: %w", output, err)
				}
			}

			findings = append(findings, finding)
			return writeJSONFile(output, findings)
		},
	}
}

func cmdCompileFindings() *serpent.Command {
	var dir, output string
	return &serpent.Command{
		Use:   "compile-findings",
		Short: "Compile per-reviewer findings into a single report.",
		Options: serpent.OptionSet{
			{
				Name:        "dir",
				Description: "Directory containing per-reviewer JSON files.",
				Flag:        "dir",
				Required:    true,
				Value:       serpent.StringOf(&dir),
			},
			{
				Name:        "output",
				Description: "Path to write the compiled output (stdout if empty).",
				Flag:        "output",
				Value:       serpent.StringOf(&output),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
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
				// Skip the output file to avoid reading our
				// own previous output on re-runs.
				if output != "" && filepath.Clean(path) == filepath.Clean(output) {
					continue
				}
				data, err := os.ReadFile(path)
				if err != nil {
					fmt.Fprintf(inv.Stderr, "Warning: could not read %s: %v\n", path, err)
					continue
				}
				data = bytes.TrimLeft(data, " \t\n\r")
				if len(data) == 0 || data[0] != '[' {
					fmt.Fprintf(inv.Stderr, "Skipping non-array JSON file: %s\n", path)
					continue
				}
				var findings []Finding
				if err := json.Unmarshal(data, &findings); err != nil {
					fmt.Fprintf(inv.Stderr, "Warning: could not parse %s: %v\n", path, err)
					continue
				}
				for _, f := range findings {
					allFindings = append(allFindings, f)
					reviewerSet[f.Reviewer] = true
				}
			}

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

			for key, findings := range groups {
				cf := buildCompiledFinding(findings)
				file := key.file
				line := key.line
				cf.File = &file
				cf.Line = &line
				compiled = append(compiled, cf)
			}

			for _, f := range noLocationFindings {
				cf := buildCompiledFinding([]Finding{f})
				compiled = append(compiled, cf)
			}

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

			bySeverity := newSeverityBuckets()
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

			return writeOutputTo(inv.Stdout, output, out)
		},
	}
}

func cmdFetchContext() *serpent.Command {
	var pr, repo, output string
	var dryRun bool
	return &serpent.Command{
		Use:   "fetch-context",
		Short: "Fetch PR context from GitHub.",
		Options: serpent.OptionSet{
			{
				Name:        "pr",
				Description: "Pull request number.",
				Flag:        "pr",
				Required:    true,
				Value:       serpent.StringOf(&pr),
			},
			{
				Name:        "repo",
				Description: "Repository in owner/name format.",
				Flag:        "repo",
				Value:       serpent.StringOf(&repo),
			},
			{
				Name:        "output",
				Description: "Path to write the output (stdout if empty).",
				Flag:        "output",
				Value:       serpent.StringOf(&output),
			},
			{
				Name:        "dry-run",
				Description: "Print commands instead of executing them.",
				Flag:        "dry-run",
				Value:       serpent.BoolOf(&dryRun),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
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
				{"pr", []string{
					"pr", "view", pr, "--repo", repo, "--json",
					"number,title,body,author,state,baseRefName,headRefName,url,headRefOid,baseRefOid",
				}},
				{"reviews", []string{"pr", "view", pr, "--repo", repo, "--json", "reviews"}},
				{"comments", []string{"pr", "view", pr, "--repo", repo, "--json", "comments"}},
				{"commits", []string{"pr", "view", pr, "--repo", repo, "--json", "commits"}},
				{"review_comments", []string{
					"api", "--paginate",
					fmt.Sprintf("repos/%s/pulls/%s/comments", repo, pr),
				}},
			}

			if dryRun {
				for _, f := range fetches {
					fmt.Fprintf(inv.Stdout, "gh %s\n", strings.Join(f.args, " "))
				}
				fmt.Fprintf(inv.Stdout, "gh api graphql -f query='<reviewThreads query for %s/%s#%s>'\n", owner, repoName, pr)
				return nil
			}

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

			reviewComments, err := filterResolvedThreads(
				results["review_comments"],
				results["threads"],
			)
			if err != nil {
				return fmt.Errorf("filtering threads: %w", err)
			}

			assembled := map[string]json.RawMessage{
				"pr":              results["pr"],
				"reviews":         extractField(results["reviews"], "reviews"),
				"review_comments": reviewComments,
				"issue_comments":  extractField(results["comments"], "comments"),
				"commits":         extractField(results["commits"], "commits"),
			}

			return writeOutputTo(inv.Stdout, output, assembled)
		},
	}
}

func cmdPostReview() *serpent.Command {
	var input, pr, repo string
	var dryRun bool
	return &serpent.Command{
		Use:   "post-review",
		Short: "Post a review to a GitHub pull request.",
		Options: serpent.OptionSet{
			{
				Name:        "input",
				Description: "Path to the review JSON file.",
				Flag:        "input",
				Required:    true,
				Value:       serpent.StringOf(&input),
			},
			{
				Name:        "pr",
				Description: "Pull request number.",
				Flag:        "pr",
				Required:    true,
				Value:       serpent.StringOf(&pr),
			},
			{
				Name:        "repo",
				Description: "Repository in owner/name format.",
				Flag:        "repo",
				Value:       serpent.StringOf(&repo),
			},
			{
				Name:        "dry-run",
				Description: "Print the payload instead of posting.",
				Flag:        "dry-run",
				Value:       serpent.BoolOf(&dryRun),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
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

			endpoint := fmt.Sprintf("repos/%s/pulls/%s/reviews", repo, pr)

			if dryRun {
				fmt.Fprintf(inv.Stdout, "POST %s\n", endpoint)
				b, err := marshalJSON(payload)
				if err != nil {
					return err
				}
				_, _ = inv.Stdout.Write(b)
				for _, r := range review.Replies {
					fmt.Fprintf(inv.Stdout, "\nREPLY to %d\n%s\n", r.InReplyToID, r.Body)
				}
				for _, tid := range review.ResolveThreadIDs {
					fmt.Fprintf(inv.Stdout, "\nRESOLVE thread %s\n", tid)
				}
				return nil
			}

			// Write payload to a temp file for gh api --input.
			payloadJSON, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("marshaling payload: %w", err)
			}
			tmpFile, err := os.CreateTemp("", "review-*.json")
			if err != nil {
				return err
			}
			defer os.Remove(tmpFile.Name())
			if _, err := tmpFile.Write(payloadJSON); err != nil {
				tmpFile.Close()
				return err
			}
			tmpFile.Close()

			if _, err := runGh("api", "-X", "POST", endpoint, "--input", tmpFile.Name()); err != nil {
				return fmt.Errorf("posting review: %w", err)
			}

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
					query := `mutation($threadId: ID!) { resolveReviewThread(input: {threadId: $threadId}) { thread { isResolved } } }`
					_, err := runGh("api", "graphql", "-f", "query="+query, "-f", "threadId="+tid)
					if err != nil {
						return fmt.Errorf("resolving thread %s: %w", tid, err)
					}
					return nil
				})
			}

			return eg.Wait()
		},
	}
}

func cmdBuildReview() *serpent.Command {
	return &serpent.Command{
		Use:   "build-review",
		Short: "Incrementally build a review JSON file.",
		Children: []*serpent.Command{
			cmdBuildReviewInit(),
			cmdBuildReviewComment(),
			cmdBuildReviewReply(),
			cmdBuildReviewResolve(),
		},
	}
}

func cmdBuildReviewInit() *serpent.Command {
	var output, body, event string
	return &serpent.Command{
		Use:   "init",
		Short: "Initialize a new review file.",
		Options: serpent.OptionSet{
			{
				Name:        "output",
				Description: "Path to the review JSON file.",
				Flag:        "output",
				Required:    true,
				Value:       serpent.StringOf(&output),
			},
			{
				Name:        "body",
				Description: "Review body text.",
				Flag:        "body",
				Required:    true,
				Value:       serpent.StringOf(&body),
			},
			{
				Name:        "event",
				Description: "Review event type.",
				Flag:        "event",
				Default:     "COMMENT",
				Value:       serpent.EnumOf(&event, "COMMENT"),
			},
		},
		Handler: func(_ *serpent.Invocation) error {
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

			return writeJSONFile(output, review)
		},
	}
}

func cmdBuildReviewComment() *serpent.Command {
	var output, path, line, body string
	return &serpent.Command{
		Use:   "comment",
		Short: "Add an inline comment to a review.",
		Options: serpent.OptionSet{
			{
				Name:        "output",
				Description: "Path to the review JSON file.",
				Flag:        "output",
				Required:    true,
				Value:       serpent.StringOf(&output),
			},
			{
				Name:        "path",
				Description: "File path for the comment.",
				Flag:        "path",
				Required:    true,
				Value:       serpent.StringOf(&path),
			},
			{
				Name:        "line",
				Description: "Line number for the comment.",
				Flag:        "line",
				Required:    true,
				Value:       serpent.StringOf(&line),
			},
			{
				Name:        "body",
				Description: "Comment body text.",
				Flag:        "body",
				Required:    true,
				Value:       serpent.StringOf(&body),
			},
		},
		Handler: func(_ *serpent.Invocation) error {
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
		},
	}
}

func cmdBuildReviewReply() *serpent.Command {
	var output, inReplyTo, body string
	return &serpent.Command{
		Use:   "reply",
		Short: "Add a reply to an existing review thread.",
		Options: serpent.OptionSet{
			{
				Name:        "output",
				Description: "Path to the review JSON file.",
				Flag:        "output",
				Required:    true,
				Value:       serpent.StringOf(&output),
			},
			{
				Name:        "in-reply-to",
				Description: "ID of the comment to reply to.",
				Flag:        "in-reply-to",
				Required:    true,
				Value:       serpent.StringOf(&inReplyTo),
			},
			{
				Name:        "body",
				Description: "Reply body text.",
				Flag:        "body",
				Required:    true,
				Value:       serpent.StringOf(&body),
			},
		},
		Handler: func(_ *serpent.Invocation) error {
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
		},
	}
}

func cmdBuildReviewResolve() *serpent.Command {
	var output, threadID string
	return &serpent.Command{
		Use:   "resolve",
		Short: "Mark a review thread for resolution.",
		Options: serpent.OptionSet{
			{
				Name:        "output",
				Description: "Path to the review JSON file.",
				Flag:        "output",
				Required:    true,
				Value:       serpent.StringOf(&output),
			},
			{
				Name:        "thread-id",
				Description: "GraphQL thread ID to resolve.",
				Flag:        "thread-id",
				Required:    true,
				Value:       serpent.StringOf(&threadID),
			},
		},
		Handler: func(_ *serpent.Invocation) error {
			return modifyReview(output, func(r *Review) {
				r.ResolveThreadIDs = append(r.ResolveThreadIDs, threadID)
			})
		},
	}
}

func modifyReview(path string, fn func(*Review)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading review file: %w (did you run 'build-review init' first?)", err)
	}
	var review Review
	if err := json.Unmarshal(data, &review); err != nil {
		return fmt.Errorf("parsing review file: %w", err)
	}
	fn(&review)
	return writeJSONFile(path, &review)
}

func buildCompiledFinding(findings []Finding) CompiledFinding {
	var reviewers []ReviewerFinding
	reviewerNames := map[string]bool{}
	maxSev := Nit
	bestSummary := ""

	for i, f := range findings {
		reviewers = append(reviewers, ReviewerFinding{
			Role:     f.Reviewer,
			Severity: f.Severity,
			Summary:  f.Summary,
			Evidence: f.Evidence,
		})
		reviewerNames[f.Reviewer] = true
		// Use first finding as default summary, then upgrade when
		// a more severe finding is seen.
		if i == 0 || f.Severity.Rank() < maxSev.Rank() {
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

func splitRepo(repo string) (string, string) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return repo, ""
	}
	return owner, name
}

func fetchReviewThreads(owner, repoName, pr string) (json.RawMessage, error) {
	var allThreads []json.RawMessage
	cursor := ""

	query := `query($owner: String!, $name: String!, $pr: Int!, $cursor: String) {
		repository(owner: $owner, name: $name) {
			pullRequest(number: $pr) {
				reviewThreads(first: 100, after: $cursor) {
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
	}`

	for {
		args := []string{
			"api", "graphql",
			"-f", "owner=" + owner,
			"-f", "name=" + repoName,
			"-F", "pr=" + pr,
			"-f", "query=" + query,
		}
		if cursor != "" {
			args = append(args, "-f", "cursor="+cursor)
		}

		out, err := runGh(args...)
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

func filterResolvedThreads(commentsRaw, threadsRaw json.RawMessage) (json.RawMessage, error) {
	if commentsRaw == nil {
		return json.RawMessage("[]"), nil
	}

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
	threadIDMap := map[int]string{}
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

	var comments []map[string]any
	if err := json.Unmarshal(commentsRaw, &comments); err != nil {
		return nil, fmt.Errorf("parsing comments: %w", err)
	}

	var filtered []map[string]any
	for _, c := range comments {
		commentID := jsonInt(c, "id")
		inReplyTo := jsonInt(c, "in_reply_to_id")

		rootID := commentID
		if inReplyTo != 0 {
			rootID = inReplyTo
		}

		if resolvedRoots[rootID] {
			continue
		}

		if tid, ok := threadIDMap[rootID]; ok {
			c["thread_id"] = tid
		}

		filtered = append(filtered, c)
	}

	if filtered == nil {
		filtered = []map[string]any{}
	}
	return json.Marshal(filtered)
}

func runGh(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func marshalJSON(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling JSON: %w", err)
	}
	data = append(data, '\n')
	return data, nil
}

func writeJSONFile(path string, v any) error {
	data, err := marshalJSON(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func writeOutputTo(w io.Writer, path string, v any) error {
	data, err := marshalJSON(v)
	if err != nil {
		return err
	}
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("creating parent directory: %w", err)
		}
		return os.WriteFile(path, data, 0o644)
	}
	_, err = w.Write(data)
	return err
}

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

func jsonInt(m map[string]any, key string) int {
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
