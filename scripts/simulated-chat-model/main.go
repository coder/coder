// Command simulated-chat-model is a development-only OpenAI-compatible model
// server for the AI workspace debugging prototype. It speaks the Chat
// Completions wire format so it can be registered in a Coder deployment as an
// "openai-compat" AI provider, which lets the real chats API, chatd worker,
// tool execution, and stream all run unchanged while the model itself is
// replaced by a rule-based diagnosis over the failure context and tool results.
//
// Every reply is prefixed with "(simulated)" so nobody mistakes it for an LLM.
//
// Usage:
//
//	go run ./scripts/simulated-chat-model --addr 127.0.0.1:18080
//
// Then create a provider with base URL http://127.0.0.1:18080/v1 and a default
// model named "simulated-debugger". See README.md in this directory.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const modelName = "simulated-debugger"

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "listen address")
	tokenDelay := flag.Duration("token-delay", 12*time.Millisecond, "delay between streamed text chunks")
	dumpContext := flag.Bool("dump-context", false, "log the failure context extracted from each request")
	flag.Parse()

	srv := &server{tokenDelay: *tokenDelay, dumpContext: *dumpContext}
	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handle)
	log.Printf("simulated chat model listening on http://%s (model %q)", *addr, modelName)
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(httpServer.ListenAndServe())
}

type server struct {
	tokenDelay  time.Duration
	dumpContext bool
}

type message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []toolCall      `json:"tool_calls,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type tool struct {
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
	} `json:"function"`
}

type request struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
	Tools    []tool    `json:"tools"`
}

// text flattens string or content-part message bodies.
func (m message) text() string {
	if len(m.Content) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(m.Content, &parts); err != nil {
		return string(m.Content)
	}
	var texts []string
	for _, p := range parts {
		if p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func (s *server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/chat/completions"):
	case strings.HasSuffix(r.URL.Path, "/responses"):
		// The chatd openai-compat client speaks Chat Completions. Refuse the
		// Responses API loudly instead of half-implementing it.
		http.Error(w, "simulated model only implements /chat/completions", http.StatusNotFound)
		return
	default:
		http.NotFound(w, r)
		return
	}

	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("%s stream=%t messages=%d tools=%d", r.URL.Path, req.Stream, len(req.Messages), len(req.Tools))

	if !req.Stream {
		// Non-streaming calls are chatd's structured "quickgen" requests,
		// such as title generation. Answer with the title schema.
		title := "Workspace failure investigation"
		if ctx := debugContextOf(req.Messages); ctx.workspaceName != "" {
			title = "Debug: " + ctx.workspaceName
		}
		writeJSON(w, completion(fmt.Sprintf(`{"title": %q}`, title)))
		return
	}

	plan := s.plan(req)
	if s.dumpContext {
		log.Printf("failure context:\n%s", debugContextOf(req.Messages).systemText)
	}
	s.stream(w, r, plan)
}

// turnPlan is what the simulated model decided to do for one request.
type turnPlan struct {
	toolName string
	toolArgs string
	text     string
}

func (*server) plan(req request) turnPlan {
	ctx := debugContextOf(req.Messages)
	last := req.Messages[len(req.Messages)-1]

	if !ctx.isDebugChat {
		return turnPlan{text: "(simulated) I am the simulated debugging model. I only know how to reason about Coder workspace failures, and this chat did not include a failure context."}
	}

	hasTool := func(name string) bool {
		for _, t := range req.Tools {
			if t.Function.Name == name {
				return true
			}
		}
		return false
	}
	sawToolResult := false
	userTurns := 0
	for _, m := range req.Messages {
		switch m.Role {
		case "tool":
			sawToolResult = true
		case "user":
			userTurns++
		}
	}

	// First turn: fetch the full build log through the real tool path so the
	// UI shows a tool call and the diagnosis can quote lines outside the tail.
	if userTurns == 1 && !sawToolResult && ctx.buildID != "" && hasTool("get_workspace_build_logs") {
		args, _ := json.Marshal(map[string]string{"build_id": ctx.buildID})
		return turnPlan{toolName: "get_workspace_build_logs", toolArgs: string(args)}
	}

	if last.Role == "tool" || userTurns == 1 {
		evidence := ctx.systemText
		for _, m := range req.Messages {
			if m.Role == "tool" {
				evidence += "\n" + m.text()
			}
		}
		return turnPlan{text: diagnose(ctx, evidence)}
	}

	return turnPlan{text: followUp(ctx, last.text())}
}

// debugContext is what the simulated model extracts from the system prompt
// written by coderd/x/workspacedebug.
type debugContext struct {
	isDebugChat     bool
	systemText      string
	workspaceName   string
	buildID         string
	buildNumber     string
	templateName    string
	jobError        string
	canEditTemplate bool
	sourceDenied    bool
	activeDiffers   bool
	noAgents        bool
}

var (
	reWorkspaceName = regexp.MustCompile(`(?m)^- name: (.+)$`)
	reBuildID       = regexp.MustCompile(`(?m)^- build_id: ([0-9a-f-]{36})`)
	reBuildNumber   = regexp.MustCompile(`(?m)^- build_number: (\d+)`)
	reTemplate      = regexp.MustCompile(`(?m)^- template: (\S+)`)
	reJobError      = regexp.MustCompile(`(?ms)^- job_error: \|\n(.*?)\n\n`)
	reCanEdit       = regexp.MustCompile(`(?m)^- can_edit_template: (true|false)`)
)

func debugContextOf(messages []message) debugContext {
	var ctx debugContext
	var sys []string
	for _, m := range messages {
		if m.Role == "system" {
			sys = append(sys, m.text())
		}
	}
	ctx.systemText = strings.Join(sys, "\n")
	marker := "# Workspace failure context"
	idx := strings.Index(ctx.systemText, marker)
	if idx < 0 {
		return ctx
	}
	ctx.isDebugChat = true
	// Only the failure bundle is evidence. The deployment system prompt
	// that precedes it mentions errors and limits in the abstract and would
	// otherwise trip the pattern matcher.
	ctx.systemText = ctx.systemText[idx:]
	if m := reWorkspaceName.FindStringSubmatch(ctx.systemText); m != nil {
		ctx.workspaceName = strings.TrimSpace(m[1])
	}
	if m := reBuildID.FindStringSubmatch(ctx.systemText); m != nil {
		ctx.buildID = m[1]
	}
	if m := reBuildNumber.FindStringSubmatch(ctx.systemText); m != nil {
		ctx.buildNumber = m[1]
	}
	if m := reTemplate.FindStringSubmatch(ctx.systemText); m != nil {
		ctx.templateName = m[1]
	}
	if m := reJobError.FindStringSubmatch(ctx.systemText); m != nil {
		ctx.jobError = strings.TrimSpace(unindent(m[1]))
	}
	if m := reCanEdit.FindStringSubmatch(ctx.systemText); m != nil {
		ctx.canEditTemplate = m[1] == "true"
	}
	ctx.sourceDenied = strings.Contains(ctx.systemText, "not permitted to read this template's source")
	ctx.activeDiffers = strings.Contains(ctx.systemText, "differs from the version this build used")
	ctx.noAgents = strings.Contains(ctx.systemText, "No agents exist for this build")
	return ctx
}

// cause is one recognized failure class.
type cause struct {
	name      string
	pattern   *regexp.Regexp
	what      string
	userFix   string
	adminFix  string
	needAdmin bool
}

var causes = []cause{
	{
		name:      "image pull",
		pattern:   regexp.MustCompile(`(?i)(pull access denied|manifest unknown|manifest for .* not found|repository does not exist|no such image|unable to find image|unable to pull image|error pulling image|failed to pull|error response from daemon: .*(not found|denied)|toomanyrequests|denied: requested access)`),
		what:      "The container image referenced by the template could not be pulled, so Terraform could not create the workspace container.",
		userFix:   "If the image is a workspace parameter, change it to one that exists and retry the build. Otherwise this is not something you can fix from the workspace.",
		adminFix:  "A template administrator needs to correct the image reference or tag in the template's `docker_image` / `coder_...` resource, or add registry credentials to the provisioner host, then push a new template version.",
		needAdmin: true,
	},
	{
		name:      "credentials",
		pattern:   regexp.MustCompile(`(?i)(unauthorized|status code: 40[13]|permission denied|accessdenied|access denied|invalid credentials|token (has )?expired|authentication (failed|required)|no valid credential)`),
		what:      "The provisioner was denied access by an upstream provider or API while creating resources.",
		userFix:   "If the template uses external authentication (GitHub, cloud provider), re-authenticate from your account settings and retry the build.",
		adminFix:  "If the error is about provider credentials on the provisioner host, a template administrator must rotate or re-grant them.",
		needAdmin: false,
	},
	{
		name:      "quota",
		pattern:   regexp.MustCompile(`(?i)(quota|limit exceeded|limitexceeded|insufficient (capacity|memory|cpu|quota)|resourceexhausted|no space left|out of memory|cannot allocate|too many)`),
		what:      "The infrastructure refused the build because a capacity or quota limit was hit.",
		userFix:   "Stop or delete workspaces you no longer need to free quota, or pick a smaller resource size parameter if the template offers one, then retry.",
		adminFix:  "If quota is at the deployment or cloud-account level, an administrator needs to raise it or add capacity.",
		needAdmin: false,
	},
	{
		name:      "agent timeout",
		pattern:   regexp.MustCompile(`(?i)(timed out waiting for (the )?agent|agent.*(never|did not) connect|start_timeout|startup script (timed out|exceeded))`),
		what:      "Resources were created but the Coder agent inside the workspace never connected, or its startup script did not finish in time.",
		userFix:   "Check your startup script and dotfiles for commands that hang or wait for input, then restart the workspace. If your personalize script is heavy, move slow steps into the background.",
		adminFix:  "If the agent binary cannot reach the Coder access URL from inside the workspace network, a template administrator has to fix the template's `coder_agent` init script or the network egress rules.",
		needAdmin: false,
	},
	{
		name:      "startup script",
		pattern:   regexp.MustCompile(`(?i)(exit status [1-9]\d*|startup script.*(failed|error)|start_error|command not found|no such file or directory|permission denied.*\.sh)`),
		what:      "The workspace was created, but a startup script exited with an error.",
		userFix:   "Open the startup log for the failing script above, fix the command that returned a non-zero exit code (often a missing binary, a bad path, or a dotfiles step), then restart the workspace.",
		adminFix:  "If the failing script is part of the template rather than your dotfiles or personalize file, a template administrator needs to change it.",
		needAdmin: false,
	},
	{
		name:      "canceled",
		pattern:   regexp.MustCompile(`(?i)cancel+ed`),
		what:      "The build was canceled before it completed.",
		userFix:   "Retry the build. If it was canceled by a timeout rather than a person, check whether the previous build was still running.",
		adminFix:  "",
		needAdmin: false,
	},
	{
		name:      "terraform",
		pattern:   regexp.MustCompile(`(?im)^(\[.*\] )?(ERROR: )?(Error:|╷|│ Error:)`),
		what:      "Terraform reported an error while planning or applying the template.",
		userFix:   "Retry once in case the failure is transient. If the same error repeats, the template itself needs a change.",
		adminFix:  "A template administrator needs to fix the resource named in the Terraform error and push a new template version; then update your workspace to the new version.",
		needAdmin: true,
	},
}

var reErrorLine = regexp.MustCompile(`(?im)^.*(error|failed|denied|not found|timed out|exit status).*$`)

func diagnose(ctx debugContext, evidence string) string {
	var matched *cause
	for i := range causes {
		if causes[i].pattern.MatchString(evidence) {
			matched = &causes[i]
			break
		}
	}

	// Evidence lines: prefer the job error, then error-looking log lines.
	var quotes []string
	if ctx.jobError != "" {
		quotes = append(quotes, firstLines(ctx.jobError, 3)...)
	}
	for _, line := range reErrorLine.FindAllString(evidence, -1) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "#") || strings.Contains(line, "Lines that look like") {
			continue
		}
		if len(quotes) >= 4 {
			break
		}
		dup := false
		for _, q := range quotes {
			if q == line {
				dup = true
				break
			}
		}
		if !dup {
			quotes = append(quotes, line)
		}
	}

	var sb strings.Builder
	_, _ = sb.WriteString("_(simulated) This diagnosis comes from the simulated model server's rule-based analysis of the build logs, not from an LLM._\n\n")

	_, _ = sb.WriteString("### What failed\n\n")
	if matched != nil {
		_, _ = sb.WriteString(matched.what)
	} else {
		_, _ = sb.WriteString("The build for workspace **" + ctx.workspaceName + "** failed, but the logs do not match a failure pattern I recognize.")
	}
	if ctx.noAgents {
		_, _ = sb.WriteString(" No resources or agents were created, so this happened during provisioning rather than inside the workspace.")
	}
	_, _ = sb.WriteString("\n\n### Evidence\n\n")
	if len(quotes) == 0 {
		_, _ = sb.WriteString("No error lines were found in the provisioner log tail; the job error above is the only signal.\n")
	} else {
		_, _ = sb.WriteString("```\n")
		for _, q := range quotes {
			_, _ = sb.WriteString(q + "\n")
		}
		_, _ = sb.WriteString("```\n")
	}

	_, _ = sb.WriteString("\n### Fix\n\n")
	if matched != nil {
		_, _ = sb.WriteString("- **You can try:** " + matched.userFix + "\n")
		if matched.adminFix != "" {
			_, _ = sb.WriteString("- **Needs a template admin:** " + matched.adminFix + "\n")
		}
	} else {
		_, _ = sb.WriteString("- **You can try:** retry the build once; transient provider errors are common.\n")
		_, _ = sb.WriteString("- **Then:** if it fails again with the same lines, share them with a template administrator.\n")
	}
	if ctx.activeDiffers {
		_, _ = sb.WriteString("- **Also:** this build used an older template version than the current active one. Use **Update** on the workspace to pick up the newest version before retrying; the fix may already be published.\n")
	}

	_, _ = sb.WriteString("\n### Who\n\n")
	needAdmin := matched != nil && matched.needAdmin
	switch {
	case needAdmin && ctx.canEditTemplate:
		_, _ = sb.WriteString("You have template-admin permissions, so you can make the template change yourself: edit the template source, push a new version, and update this workspace.\n")
	case needAdmin || (matched == nil && ctx.sourceDenied):
		_, _ = sb.WriteString("**You need an administrator.** ")
		if ctx.sourceDenied {
			_, _ = sb.WriteString("You cannot read this template's source, so the fix is outside your control. ")
		}
		_, _ = sb.WriteString("Send them this:\n\n")
		_, _ = fmt.Fprintf(&sb, "> Workspace `%s` (template `%s`, build #%s) fails to start", ctx.workspaceName, ctx.templateName, ctx.buildNumber)
		if len(quotes) > 0 {
			_, _ = fmt.Fprintf(&sb, " with `%s`", truncate(quotes[0], 160))
		}
		_, _ = sb.WriteString(". Could you check the template version this build used and push a fix? Build ID `" + ctx.buildID + "`.\n")
	default:
		_, _ = sb.WriteString("You can most likely resolve this yourself with the steps above. If a retry fails the same way, escalate to a template administrator with the build ID `" + ctx.buildID + "`.\n")
	}
	return sb.String()
}

func followUp(ctx debugContext, question string) string {
	q := truncate(strings.TrimSpace(question), 200)
	var sb strings.Builder
	_, _ = sb.WriteString("_(simulated) Follow-up answers are canned; a real model would reason about your question._\n\n")
	_, _ = fmt.Fprintf(&sb, "You asked: \"%s\"\n\n", q)
	lower := strings.ToLower(q)
	switch {
	case strings.Contains(lower, "retry") || strings.Contains(lower, "again"):
		_, _ = sb.WriteString("Retrying is safe: a failed build leaves no resources behind, so use **Start** (or **Retry**) on the workspace page. If the same error line appears in the new build's logs, the failure is deterministic and the fix has to happen in the template or your parameters.\n")
	case strings.Contains(lower, "admin") || strings.Contains(lower, "who"):
		_, _ = fmt.Fprintf(&sb, "Template `%s` is owned by your organization's template administrators. The Templates page lists the template; its **Settings** tab shows who last pushed a version. Share build ID `%s` with them.\n", ctx.templateName, ctx.buildID)
	case strings.Contains(lower, "log"):
		_, _ = fmt.Fprintf(&sb, "The full provisioner log is on the left of this page and is also available through `get_workspace_build_logs` with build ID `%s`. I have already read it once for the diagnosis above.\n", ctx.buildID)
	default:
		_, _ = sb.WriteString("The diagnosis above still stands. The quickest next step is the first item under **Fix**; if it does not work, paste the new build's error line here and I will re-check it.\n")
	}
	return sb.String()
}

// stream writes the plan as Chat Completions SSE chunks.
func (s *server) stream(w http.ResponseWriter, r *http.Request, plan turnPlan) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	id := "chatcmpl-" + uuid.NewString()[:8]
	created := time.Now().Unix()

	write := func(choice map[string]any) bool {
		chunk := map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   modelName,
			"choices": []map[string]any{choice},
		}
		data, err := json.Marshal(chunk)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if plan.toolName != "" {
		write(map[string]any{
			"index": 0,
			"delta": map[string]any{
				"role": "assistant",
				"tool_calls": []map[string]any{{
					"index": 0,
					"id":    "call_" + uuid.NewString()[:8],
					"type":  "function",
					"function": map[string]any{
						"name":      plan.toolName,
						"arguments": plan.toolArgs,
					},
				}},
			},
		})
		write(map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	write(map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": ""}})
	for _, piece := range splitForStreaming(plan.text) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(s.tokenDelay):
		}
		if !write(map[string]any{"index": 0, "delta": map[string]any{"content": piece}}) {
			return
		}
	}
	write(map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"})
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// splitForStreaming chunks text on whitespace so the UI shows it arriving.
func splitForStreaming(text string) []string {
	var pieces []string
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == ' ' || text[i] == '\n' {
			pieces = append(pieces, text[start:i+1])
			start = i + 1
		}
	}
	if start < len(text) {
		pieces = append(pieces, text[start:])
	}
	return pieces
}

func completion(text string) map[string]any {
	return map[string]any{
		"id":      "chatcmpl-" + uuid.NewString()[:8],
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   modelName,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": text},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func firstLines(s string, n int) []string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return lines
}

func unindent(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimPrefix(lines[i], "    ")
	}
	return strings.Join(lines, "\n")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
