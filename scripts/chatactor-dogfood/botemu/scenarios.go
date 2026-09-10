package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

type verdict string

const (
	pass    verdict = "PASS"
	fail    verdict = "FAIL"
	skipped verdict = "SKIPPED"
)

type scenarioResult struct {
	ID       string
	Name     string
	Verdict  verdict
	Evidence []string
}

type reporter struct {
	results []*scenarioResult
	cur     *scenarioResult
}

func (r *reporter) begin(id, name string) {
	r.cur = &scenarioResult{ID: id, Name: name, Verdict: pass}
	r.results = append(r.results, r.cur)
	_, _ = fmt.Printf("\n=== %s %s ===\n", id, name)
}

func (r *reporter) ev(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	r.cur.Evidence = append(r.cur.Evidence, line)
	_, _ = fmt.Println("  " + strings.ReplaceAll(line, "\n", "\n  "))
}

// check records a PASS/FAIL assertion line.
func (r *reporter) check(ok bool, format string, args ...any) bool { //nolint:revive // ok is the assertion result, not a control flag.
	prefix := "ok:   "
	if !ok {
		prefix = "FAIL: "
		r.cur.Verdict = fail
	}
	r.ev(prefix+format, args...)
	return ok
}

func (r *reporter) skip(format string, args ...any) {
	r.cur.Verdict = skipped
	r.ev("skipped: "+format, args...)
}

func (r *reporter) failf(format string, args ...any) {
	r.cur.Verdict = fail
	r.ev("FAIL: "+format, args...)
}

func (r *reporter) write(path string) error {
	var b strings.Builder
	_, _ = b.WriteString("# botemu results\n\n")
	_, _ = fmt.Fprintf(&b, "Generated %s\n\n", time.Now().Format(time.RFC3339))
	_, _ = b.WriteString("| Scenario | Verdict |\n|---|---|\n")
	for _, res := range r.results {
		_, _ = fmt.Fprintf(&b, "| %s %s | %s |\n", res.ID, res.Name, res.Verdict)
	}
	_, _ = b.WriteString("\n")
	for _, res := range r.results {
		_, _ = fmt.Fprintf(&b, "## %s %s: %s\n\n```text\n", res.ID, res.Name, res.Verdict)
		for _, e := range res.Evidence {
			_, _ = b.WriteString(e)
			_, _ = b.WriteString("\n")
		}
		_, _ = b.WriteString("```\n\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func runScenarios(ctx context.Context, st *state, only string, withS9 bool, newChat bool) error {
	want := map[string]bool{}
	if only != "" {
		for _, id := range strings.Split(only, ",") {
			want[strings.TrimSpace(strings.ToUpper(id))] = true
		}
	}
	should := func(id string) bool {
		if len(want) == 0 {
			if id == "S4A" {
				return false
			}
			if id == "S9B" {
				return false
			}
			return id != "S9" || withS9
		}
		return want[id]
	}
	r := &reporter{}
	steps := []struct {
		id, name string
		fn       func(context.Context, *state, *reporter) error
	}{
		{"S0", "Discovery", s0Discovery},
		{"S1", "Token identity", s1TokenIdentity},
		{"S2", "Thread start", func(ctx context.Context, st *state, r *reporter) error { return s2ThreadStart(ctx, st, r, newChat) }},
		{"S3", "Join ordering and sharing", s3JoinOrdering},
		{"S4A", "MCP whoami smoke (alice only)", func(ctx context.Context, st *state, r *reporter) error {
			return mcpIdentityFor(ctx, st, r, []string{"alice"})
		}},
		{"S4", "Per-user MCP identity", s4MCPIdentity},
		{"S5", "Workspace denial", s5WorkspaceDenial},
		{"S6", "Share and retry", s6ShareAndRetry},
		{"S7", "Handler gates", s7HandlerGates},
		{"S8", "Attribution", s8Attribution},
		{"S9", "Create workspace from chat (optional)", s9CreateWorkspace},
		{"S9B", "Use sharer creates workspace (unbound chat)", func(ctx context.Context, st *state, r *reporter) error {
			return s9bUseSharerCreatesWorkspace(ctx, st, r, newChat)
		}},
	}
	for _, s := range steps {
		if !should(s.id) {
			continue
		}
		r.begin(s.id, s.name)
		if err := s.fn(ctx, st, r); err != nil {
			r.failf("scenario error: %v", err)
		}
		_, _ = fmt.Printf("=== %s verdict: %s ===\n", s.id, r.cur.Verdict)
		_ = st.save()
	}
	_, _ = fmt.Println("\nSummary:")
	for _, res := range r.results {
		_, _ = fmt.Printf("  %s %-40s %s\n", res.ID, res.Name, res.Verdict)
	}
	return r.write(resultsFile)
}

func (s *state) tokenClient(name string) *codersdk.Client {
	return s.client(s.Users[name].AccessToken)
}

// ---------------------------------------------------------------- S0

func s0Discovery(ctx context.Context, st *state, r *reporter) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, st.AccessURL+"/.well-known/oauth-authorization-server", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var meta struct {
		Issuer          string   `json:"issuer"`
		TokenEndpoint   string   `json:"token_endpoint"`
		ScopesSupported []string `json:"scopes_supported"`
	}
	if err := decodeJSON(resp.Body, &meta); err != nil {
		return err
	}
	r.ev("GET /.well-known/oauth-authorization-server -> %d issuer=%s token_endpoint=%s", resp.StatusCode, meta.Issuer, meta.TokenEndpoint)
	have := map[string]bool{}
	for _, s := range meta.ScopesSupported {
		have[s] = true
	}
	var chatScopes []string
	for _, s := range meta.ScopesSupported {
		if strings.HasPrefix(s, "chat") || s == "workspace:share" {
			chatScopes = append(chatScopes, s)
		}
	}
	r.ev("scopes_supported (chat*/workspace:share subset): %v", chatScopes)
	for _, s := range []string{"chat:read", "chat:create", "chat:use", "chat:update", "chat:delete", "chat:share", "chat:*", "workspace:share"} {
		r.check(have[s], "scopes_supported includes %s", s)
	}
	return nil
}

// ---------------------------------------------------------------- S1

func s1TokenIdentity(ctx context.Context, st *state, r *reporter) error {
	for _, name := range []string{"alice", "bob", "carol"} {
		us := st.Users[name]
		for _, hdr := range []string{"Authorization: Bearer", "Coder-Session-Token"} {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, st.AccessURL+"/api/v2/users/me", nil)
			if hdr == "Coder-Session-Token" {
				req.Header.Set("Coder-Session-Token", us.AccessToken)
			} else {
				req.Header.Set("Authorization", "Bearer "+us.AccessToken)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			var u struct {
				ID       uuid.UUID `json:"id"`
				Username string    `json:"username"`
				Message  string    `json:"message"`
				Detail   string    `json:"detail"`
			}
			_ = unmarshalJSON(body, &u)
			if resp.StatusCode == http.StatusOK {
				r.check(u.ID == us.ID && u.Username == name, "%s via %s: GET /api/v2/users/me -> %d id=%s username=%s (expected %s)", name, hdr, resp.StatusCode, u.ID, u.Username, us.ID)
			} else {
				r.check(false, "%s via %s: GET /api/v2/users/me -> %d message=%q detail=%q (token scopes=%q)", name, hdr, resp.StatusCode, u.Message, u.Detail, us.Scopes)
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------- S2

func s2ThreadStart(ctx context.Context, st *state, r *reporter, newChat bool) error { //nolint:revive // newChat mirrors the -new-chat flag.
	alice := st.tokenClient("alice")
	if !newChat && st.ChatID != uuid.Nil {
		chat, err := alice.GetChat(ctx, st.ChatID)
		if err != nil {
			return err
		}
		r.ev("reusing chat %s status=%s workspace_id=%s agent_id=%s", chat.ID, chat.Status, uuidStr(chat.WorkspaceID), uuidStr(chat.AgentID))
		return nil
	}
	ws := st.WorkspaceID
	chat, err := alice.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: st.OrgID,
		WorkspaceID:    &ws,
		SystemPrompt:   systemPrompt,
		ClientType:     codersdk.ChatClientTypeAPI,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Reply with the single word pong."}},
	})
	if err != nil {
		r.failf("alice token POST /api/v2/chats failed: %s", errBody(err))
		return nil
	}
	st.ChatID = chat.ID
	_ = st.save()
	r.ev("POST /api/v2/chats -> chat id=%s owner_id=%s workspace_id=%s agent_id=%s status=%s mcp_server_ids=%v client_type=%s",
		chat.ID, chat.OwnerID, uuidStr(chat.WorkspaceID), uuidStr(chat.AgentID), chat.Status, chat.MCPServerIDs, chat.ClientType)
	r.check(chat.OwnerID == st.Users["alice"].ID, "chat owner is alice (%s)", st.Users["alice"].ID)
	r.check(chat.WorkspaceID != nil && *chat.WorkspaceID == st.WorkspaceID, "chat bound to alice's workspace %s", st.WorkspaceID)
	hasMCP := false
	for _, id := range chat.MCPServerIDs {
		if id == st.MCPConfigID {
			hasMCP = true
		}
	}
	r.check(hasMCP, "force_on MCP config %s attached to chat", st.MCPConfigID)

	t, err := waitTurn(ctx, alice, chat.ID, codersdk.ChatMessage{ID: 0, CreatedAt: time.Now()}, 2*time.Minute)
	if err != nil {
		r.failf("turn did not complete: %v", err)
		return nil
	}
	// The first message is alice's prompt.
	for _, m := range t.Messages {
		if m.Role == codersdk.ChatMessageRoleUser {
			t.UserMessage = m
			break
		}
	}
	r.ev("turn: %s", describeTurn(t))
	r.check(t.UserMessage.CreatedBy != nil && *t.UserMessage.CreatedBy == st.Users["alice"].ID, "first user message created_by == alice")
	r.check(t.Chat.Status == codersdk.ChatStatusWaiting, "chat status is waiting after turn (got %s)", t.Chat.Status)
	r.check(strings.Contains(strings.ToLower(t.assistantText()), "pong"), "assistant replied (contains 'pong')")
	// The agent binding is resolved by chatd when the turn runs.
	r.check(t.Chat.AgentID != nil && *t.Chat.AgentID == st.AgentID, "chat agent_id after turn == %s (got %s)", st.AgentID, uuidStr(t.Chat.AgentID))
	return nil
}

// ---------------------------------------------------------------- S3

func s3JoinOrdering(ctx context.Context, st *state, r *reporter) error {
	if st.ChatID == uuid.Nil {
		r.skip("no chat (S2 did not run)")
		return nil
	}
	bob := st.tokenClient("bob")
	_, err := bob.CreateChatMessage(ctx, st.ChatID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Reply with the single word hi."}},
	})
	code := httpStatus(err)
	r.check(err != nil && (code == 403 || code == 404), "bob POST /chats/{id}/messages before any grant denied: %s", errBody(err))

	aliceNoShare := st.client(st.AliceNoShareTk)
	err = aliceNoShare.UpdateChatACL(ctx, st.ChatID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{st.Users["bob"].ID.String(): codersdk.ChatRoleUse},
	})
	r.check(httpStatus(err) == 403, "alice token WITHOUT chat:share PATCH /chats/{id}/acl -> %s", errBody(err))

	alice := st.tokenClient("alice")
	err = alice.UpdateChatACL(ctx, st.ChatID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{
			st.Users["bob"].ID.String():   codersdk.ChatRoleUse,
			st.Users["carol"].ID.String(): codersdk.ChatRoleRead,
		},
	})
	if !r.check(err == nil, "alice full token PATCH /chats/{id}/acl bob=use carol=read -> %s", errBody(err)) {
		return nil
	}
	acl, err := alice.GetChatACL(ctx, st.ChatID)
	if err != nil {
		return err
	}
	roles := map[uuid.UUID]codersdk.ChatRole{}
	var summary []string
	for _, u := range acl.Users {
		roles[u.ID] = u.Role
		summary = append(summary, fmt.Sprintf("%s=%s", u.Username, u.Role))
	}
	r.ev("GET /chats/{id}/acl users: %v groups=%d", summary, len(acl.Groups))
	r.check(roles[st.Users["bob"].ID] == codersdk.ChatRoleUse, "bob role round-trips as use")
	r.check(roles[st.Users["carol"].ID] == codersdk.ChatRoleRead, "carol role round-trips as read")
	return nil
}

// ---------------------------------------------------------------- S4

const whoamiPrompt = "Call the whoami tool and reply with its exact output."

func s4MCPIdentity(ctx context.Context, st *state, r *reporter) error {
	return mcpIdentityFor(ctx, st, r, []string{"bob", "alice"})
}

func mcpIdentityFor(ctx context.Context, st *state, r *reporter, names []string) error {
	if st.ChatID == uuid.Nil {
		r.skip("no chat")
		return nil
	}
	for _, name := range names {
		c := st.tokenClient(name)
		t, err := postAndWait(ctx, c, st.ChatID, whoamiPrompt, 2*time.Minute)
		if err != nil {
			r.failf("%s post/wait: %v (%s)", name, err, errBody(err))
			continue
		}
		r.ev("%s turn: %s", name, describeTurn(t))
		r.check(t.UserMessage.CreatedBy != nil && *t.UserMessage.CreatedBy == st.Users[name].ID, "%s user message created_by == %s (%s)", name, name, st.Users[name].ID)
		var found bool
		for _, res := range t.toolResults() {
			if !strings.HasSuffix(res.ToolName, "__whoami") && res.ToolName != "whoami" {
				continue
			}
			found = true
			txt := resultText(res)
			r.check(strings.Contains(txt, `"owner_id":"`+st.Users["alice"].ID.String()+`"`), "%s: whoami owner_id == alice (%s)", name, st.Users["alice"].ID)
			r.check(strings.Contains(txt, `"actor_id":"`+st.Users[name].ID.String()+`"`), "%s: whoami actor_id == %s (%s)", name, name, st.Users[name].ID)
			r.check(strings.Contains(txt, `"chat_id":"`+st.ChatID.String()+`"`), "%s: whoami chat_id == chat", name)
			r.check(strings.Contains(txt, `"token_label":"`+name+`"`), "%s: whoami token_label == %s (per-user MCP token follows actor)", name, name)
			r.check(res.MCPServerConfigID.Valid && res.MCPServerConfigID.UUID == st.MCPConfigID, "%s: tool-result mcp_server_config_id == %s", name, st.MCPConfigID)
		}
		r.check(found, "%s: a whoami tool-result was recorded", name)
	}
	return nil
}

// ---------------------------------------------------------------- S5

const executePrompt = "Use the execute tool to run `hostname` in the workspace and reply with the output."

func s5WorkspaceDenial(ctx context.Context, st *state, r *reporter) error {
	if st.ChatID == uuid.Nil {
		r.skip("no chat")
		return nil
	}
	bob := st.tokenClient("bob")
	t, err := postAndWait(ctx, bob, st.ChatID, executePrompt, 2*time.Minute)
	if err != nil {
		r.failf("bob post/wait: %v (%s)", err, errBody(err))
		return nil
	}
	r.ev("bob turn: %s", describeTurn(t))
	want := fmt.Sprintf("workspace access denied: user bob does not have access to workspace alice/%s", st.WorkspaceName)
	var execFound bool
	for _, res := range t.toolResults() {
		if res.ToolName != "execute" {
			continue
		}
		execFound = true
		txt := resultText(res)
		r.check(strings.Contains(txt, want), "execute tool-result contains %q", want)
		r.check(res.IsError, "execute tool-result is_error=true (got %v)", res.IsError)
	}
	r.check(execFound, "an execute tool-result was recorded")
	at := t.assistantText()
	r.check(strings.Contains(at, "does not have access") || strings.Contains(at, "access denied"), "assistant relays the denial text")
	lines := grepLog(developLog, "actor_id="+st.Users["bob"].ID.String())
	r.check(len(lines) > 0, "server log has %d line(s) with actor_id=%s (bob)", len(lines), st.Users["bob"].ID)
	if len(lines) > 0 {
		r.ev("log: %s", truncate(lines[len(lines)-1], 400))
	}
	return nil
}

// ---------------------------------------------------------------- S6

func s6ShareAndRetry(ctx context.Context, st *state, r *reporter) error {
	if st.ChatID == uuid.Nil {
		r.skip("no chat")
		return nil
	}
	alice := st.tokenClient("alice")
	err := alice.UpdateWorkspaceACL(ctx, st.WorkspaceID, codersdk.UpdateWorkspaceACL{
		UserRoles: map[string]codersdk.WorkspaceRole{st.Users["bob"].ID.String(): codersdk.WorkspaceRoleUse},
	})
	if !r.check(err == nil, "alice token PATCH /workspaces/{id}/acl bob=use -> %s", errBody(err)) {
		return nil
	}
	acl, err := alice.WorkspaceACL(ctx, st.WorkspaceID)
	if err == nil {
		var s []string
		for _, u := range acl.Users {
			s = append(s, fmt.Sprintf("%s=%s", u.Username, u.Role))
		}
		r.ev("GET /workspaces/{id}/acl users: %v", s)
	}
	bob := st.tokenClient("bob")
	t, err := postAndWait(ctx, bob, st.ChatID, executePrompt, 2*time.Minute)
	if err != nil {
		r.failf("bob post/wait: %v (%s)", err, errBody(err))
		return nil
	}
	r.ev("bob turn: %s", describeTurn(t))
	var execFound bool
	for _, res := range t.toolResults() {
		if res.ToolName != "execute" {
			continue
		}
		execFound = true
		txt := resultText(res)
		r.check(!res.IsError, "execute tool-result is_error=false (got %v)", res.IsError)
		r.check(!strings.Contains(txt, "workspace access denied"), "execute tool-result has no denial text")
		r.check(strings.Contains(txt, st.WorkspaceName), "execute output contains container hostname %q (docker template sets hostname = workspace name)", st.WorkspaceName)
	}
	r.check(execFound, "an execute tool-result was recorded")
	return nil
}

// ---------------------------------------------------------------- S7

func s7HandlerGates(ctx context.Context, st *state, r *reporter) error {
	if st.ChatID == uuid.Nil {
		r.skip("no chat")
		return nil
	}
	bob := st.tokenClient("bob")
	carol := st.tokenClient("carol")
	archived := true
	err := bob.UpdateChat(ctx, st.ChatID, codersdk.UpdateChatRequest{Archived: &archived})
	r.check(httpStatus(err) == 403 || httpStatus(err) == 404, "bob PATCH /chats/{id} archive denied: %s", errBody(err))
	err = bob.UpdateChatACL(ctx, st.ChatID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{st.Users["carol"].ID.String(): codersdk.ChatRoleUse},
	})
	r.check(httpStatus(err) == 403 || httpStatus(err) == 404, "bob PATCH /chats/{id}/acl denied: %s", errBody(err))

	// Interrupt: start a slow turn as bob, then interrupt as bob.
	resp, err := bob.CreateChatMessage(ctx, st.ChatID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Use the execute tool to run `sleep 45` in the workspace and reply with the output."}},
	})
	if err != nil {
		r.skip("interrupt: could not start a turn as bob: %s", errBody(err))
	} else {
		time.Sleep(4 * time.Second)
		chat, _ := bob.GetChat(ctx, st.ChatID)
		if chat.Status != codersdk.ChatStatusRunning {
			r.skip("interrupt: chat not running when checked (status=%s)", chat.Status)
		} else {
			ic, err := bob.InterruptChat(ctx, st.ChatID)
			r.check(err == nil, "bob POST /chats/{id}/interrupt while running -> %s status=%s", errBody(err), ic.Status)
		}
		if resp.Message != nil {
			t, werr := waitTurn(ctx, bob, st.ChatID, *resp.Message, 2*time.Minute)
			if werr == nil {
				r.ev("after interrupt: chat status=%s messages_after=%d", t.Chat.Status, len(t.Messages))
			} else {
				r.ev("after interrupt: wait error: %v", werr)
			}
		}
	}

	_, err = carol.CreateChatMessage(ctx, st.ChatID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Reply with the single word hi."}},
	})
	// Read-only sharers fail the ActionUse RBAC gate and get 404
	// (httpapi.ResourceNotFound).
	r.check(httpStatus(err) == 403 || httpStatus(err) == 404, "carol (read) POST /chats/{id}/messages denied (403 or 404): %s", errBody(err))

	admin, err := st.adminClient()
	if err != nil {
		return err
	}
	_, err = admin.CreateChatMessage(ctx, st.ChatID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Reply with the single word hi."}},
	})
	// Admins hold update on every chat but not use, so posting fails the
	// ActionUse RBAC gate with 404.
	r.check(httpStatus(err) == 404, "admin session POST /chats/{id}/messages denied with 404: %s", errBody(err))

	for _, name := range []string{"bob", "carol"} {
		c := st.tokenClient(name)
		chat, err := c.GetChat(ctx, st.ChatID)
		r.check(err == nil, "%s GET /chats/{id} -> %s (status=%s)", name, errBody(err), chat.Status)
		msgs, err := c.GetChatMessages(ctx, st.ChatID, nil)
		r.check(err == nil, "%s GET /chats/{id}/messages -> %s (%d messages)", name, errBody(err), len(msgs.Messages))
	}
	return nil
}

// ---------------------------------------------------------------- S8

func s8Attribution(ctx context.Context, st *state, r *reporter) error {
	db, err := openDevDB()
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
SELECT u.username, k.token_name, k.login_type::text, k.scopes::text[], k.expires_at
FROM api_keys k JOIN users u ON u.id = k.user_id
WHERE k.token_name LIKE 'chatd\_%\_session_token'
ORDER BY u.username`)
	if err != nil {
		return err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var username, tokenName, loginType string
		var scopes []string
		var expires time.Time
		if err := rows.Scan(&username, &tokenName, &loginType, (*pqStringArray)(&scopes), &expires); err != nil {
			return err
		}
		counts[username]++
		r.ev("api_keys row: username=%s token_name=%s login_type=%s scopes=%v expires_at=%s", username, tokenName, loginType, scopes, expires.Format(time.RFC3339))
	}
	r.check(counts["alice"] == 1, "one chatd gateway key for alice (got %d)", counts["alice"])
	r.check(counts["bob"] == 1, "one chatd gateway key for bob (got %d)", counts["bob"])
	r.check(counts["carol"] == 0, "no chatd gateway key for carol (got %d)", counts["carol"])
	r.check(counts["admin"] == 0, "no chatd gateway key for admin (got %d)", counts["admin"])

	for _, name := range []string{"alice", "bob"} {
		lines := grepLog(developLog, "actor_id="+st.Users[name].ID.String())
		r.check(len(lines) > 0, "server log has %d line(s) with actor_id=%s (%s)", len(lines), st.Users[name].ID, name)
		if len(lines) > 0 {
			r.ev("log[%s]: %s", name, truncate(lines[0], 400))
		}
	}
	return nil
}

// ---------------------------------------------------------------- S9

func s9CreateWorkspace(ctx context.Context, st *state, r *reporter) error {
	if st.ChatID == uuid.Nil {
		r.skip("no chat")
		return nil
	}
	bob := st.tokenClient("bob")
	t, err := postAndWait(ctx, bob, st.ChatID,
		"Use the create_workspace tool to create a workspace named bob-from-chat from the template named docker, then reply when it is ready.",
		10*time.Minute)
	if err != nil {
		r.failf("bob post/wait: %v (%s)", err, errBody(err))
		return nil
	}
	r.ev("bob turn: %s", describeTurn(t))
	admin, err := st.adminClient()
	if err != nil {
		return err
	}
	ws, err := admin.WorkspaceByOwnerAndName(ctx, "bob", "bob-from-chat", codersdk.WorkspaceOptions{})
	if !r.check(err == nil, "workspace bob/bob-from-chat exists: %s", errBody(err)) {
		return nil
	}
	r.ev("workspace id=%s owner=%s status=%s", ws.ID, ws.OwnerName, ws.LatestBuild.Status)
	r.check(ws.OwnerID == st.Users["bob"].ID, "workspace owner_id == bob")
	chat, err := bob.GetChat(ctx, st.ChatID)
	if err != nil {
		return err
	}
	r.check(chat.WorkspaceID != nil && *chat.WorkspaceID == ws.ID, "chat workspace_id switched to bob-from-chat (got %s)", uuidStr(chat.WorkspaceID))

	t, err = postAndWait(ctx, bob, st.ChatID, executePrompt, 3*time.Minute)
	if err != nil {
		r.failf("bob execute post/wait: %v", err)
		return nil
	}
	r.ev("bob execute turn: %s", describeTurn(t))
	for _, res := range t.toolResults() {
		if res.ToolName == "execute" {
			r.check(!res.IsError && strings.Contains(resultText(res), "bob-from-chat"), "bob execute on bob-from-chat succeeds")
		}
	}
	alice := st.tokenClient("alice")
	t, err = postAndWait(ctx, alice, st.ChatID, executePrompt, 3*time.Minute)
	if err != nil {
		r.failf("alice execute post/wait: %v", err)
		return nil
	}
	r.ev("alice execute turn: %s", describeTurn(t))
	want := "workspace access denied: user alice does not have access to workspace bob/bob-from-chat"
	var found bool
	for _, res := range t.toolResults() {
		if res.ToolName == "execute" {
			found = true
			r.check(strings.Contains(resultText(res), want), "alice execute denied with %q", want)
		}
	}
	r.check(found, "an execute tool-result was recorded for alice")
	return nil
}

// ---------------------------------------------------------------- S9B

const s9bWorkspaceName = "bob-from-chat"

// s9bUseSharerCreatesWorkspace verifies that a chat use sharer (bob) who creates a
// workspace from inside alice's UNBOUND chat ends up owning that workspace,
// and that later tool calls are checked against the actor, not the owner.
//
// With newChat=false and an S9B chat already in state, steps 1-4 are skipped
// and bob starts the (stopped) workspace before steps 5-7 run again.
func s9bUseSharerCreatesWorkspace(ctx context.Context, st *state, r *reporter, newChat bool) error { //nolint:revive // newChat mirrors the -new-chat flag.
	alice := st.tokenClient("alice")
	bob := st.tokenClient("bob")
	aliceID := st.Users["alice"].ID
	bobID := st.Users["bob"].ID
	admin, err := st.adminClient()
	if err != nil {
		return err
	}

	if !newChat && st.S9BChatID != uuid.Nil && st.S9BWorkspaceID != uuid.Nil {
		chat, err := admin.GetChat(ctx, st.S9BChatID)
		if err != nil {
			return err
		}
		ws, err := admin.Workspace(ctx, st.S9BWorkspaceID)
		if err != nil {
			return err
		}
		r.ev("resume: reusing chat %s (owner=%s workspace_id=%s) and workspace %s/%s (latest_build transition=%s status=%s)",
			chat.ID, chat.OwnerID, uuidStr(chat.WorkspaceID), ws.OwnerName, ws.Name, ws.LatestBuild.Transition, ws.LatestBuild.Status)
		if ws.LatestBuild.Transition == codersdk.WorkspaceTransitionStop {
			t, err := postAndWait(ctx, bob, chat.ID, "Use the start_workspace tool to start the workspace and reply with the result.", 10*time.Minute)
			if err != nil {
				r.failf("resume: bob start post/wait: %v", err)
				return nil
			}
			r.ev("resume: bob start turn: %s", describeTurn(t))
			var started bool
			for _, res := range t.toolResults() {
				if res.ToolName == "start_workspace" && !res.IsError && !strings.Contains(resultText(res), `"error"`) {
					started = true
				}
			}
			if !r.check(started, "resume: bob start_workspace tool-result succeeds") {
				return nil
			}
			ws, err = admin.Workspace(ctx, ws.ID)
			if err != nil {
				return err
			}
			r.check(ws.LatestBuild.Transition == codersdk.WorkspaceTransitionStart && ws.LatestBuild.InitiatorID == bobID, "resume: start build initiated by bob (transition=%s initiator_id=%s)", ws.LatestBuild.Transition, ws.LatestBuild.InitiatorID)
		}
		return s9bActorChecks(ctx, st, r, admin, chat, ws)
	}

	// Step 1: alice creates a new chat with no workspace binding.
	chat, err := alice.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: st.OrgID,
		SystemPrompt:   systemPrompt,
		ClientType:     codersdk.ChatClientTypeAPI,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Reply with the single word pong."}},
	})
	if err != nil {
		r.failf("step1: alice token POST /api/v2/chats (no workspace_id) failed: %s", errBody(err))
		return nil
	}
	st.S9BChatID = chat.ID
	_ = st.save()
	r.ev("step1: POST /api/v2/chats -> chat id=%s owner_id=%s workspace_id=%s agent_id=%s status=%s", chat.ID, chat.OwnerID, uuidStr(chat.WorkspaceID), uuidStr(chat.AgentID), chat.Status)
	r.check(chat.OwnerID == aliceID, "step1: chat owner is alice (%s)", aliceID)
	r.check(chat.WorkspaceID == nil, "step1: chat workspace_id is nil")
	t, err := waitTurn(ctx, alice, chat.ID, codersdk.ChatMessage{ID: 0, CreatedAt: time.Now()}, 2*time.Minute)
	if err != nil {
		r.failf("step1: turn did not complete: %v", err)
		return nil
	}
	for _, m := range t.Messages {
		if m.Role == codersdk.ChatMessageRoleUser {
			t.UserMessage = m
			break
		}
	}
	r.ev("step1: turn: %s", describeTurn(t))
	r.check(strings.Contains(strings.ToLower(t.assistantText()), "pong"), "step1: assistant replied (contains 'pong')")
	r.check(t.Chat.WorkspaceID == nil, "step1: chat workspace_id still nil after turn (got %s)", uuidStr(t.Chat.WorkspaceID))

	// Step 2: alice grants bob use.
	err = alice.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{bobID.String(): codersdk.ChatRoleUse},
	})
	if !r.check(err == nil, "step2: alice PATCH /chats/{id}/acl bob=use -> %s", errBody(err)) {
		return nil
	}
	acl, err := alice.GetChatACL(ctx, chat.ID)
	if err != nil {
		return err
	}
	var bobRole codersdk.ChatRole
	for _, u := range acl.Users {
		if u.ID == bobID {
			bobRole = u.Role
		}
	}
	r.check(bobRole == codersdk.ChatRoleUse, "step2: GET /chats/{id}/acl bob role round-trips as use (got %q)", bobRole)

	// Step 3: bob asks the chat to create a workspace.
	createPrompt := fmt.Sprintf("Use the create_workspace tool to create a workspace named %s from the docker template (template_id %s), then reply with the single word done.", s9bWorkspaceName, st.TemplateID)
	t, err = postAndWait(ctx, bob, chat.ID, createPrompt, 10*time.Minute)
	if err != nil {
		r.failf("step3: bob post/wait: %v (%s)", err, errBody(err))
		return nil
	}
	r.ev("step3: bob turn: %s", describeTurn(t))
	r.check(t.UserMessage.CreatedBy != nil && *t.UserMessage.CreatedBy == bobID, "step3: bob user message created_by == bob")
	var createFound bool
	for _, res := range t.toolResults() {
		if res.ToolName != "create_workspace" {
			continue
		}
		createFound = true
		txt := resultText(res)
		r.check(!res.IsError, "step3: create_workspace tool-result is_error=false (got %v)", res.IsError)
		r.check(strings.Contains(txt, `"created":true`), "step3: create_workspace tool-result has created=true")
		r.check(!strings.Contains(txt, "already_exists"), "step3: create_workspace tool-result has no already_exists")
	}
	if !r.check(createFound, "step3: a create_workspace tool-result was recorded") {
		return nil
	}

	// Step 4: admin verifies ownership, initiator, chat binding, and audit log.
	ws, err := admin.WorkspaceByOwnerAndName(ctx, "bob", s9bWorkspaceName, codersdk.WorkspaceOptions{})
	if !r.check(err == nil, "step4: GET /api/v2/users/bob/workspace/%s -> %s", s9bWorkspaceName, errBody(err)) {
		return nil
	}
	st.S9BWorkspaceID = ws.ID
	_ = st.save()
	r.ev("step4: workspace id=%s owner=%s/%s owner_id=%s latest_build id=%s initiator_id=%s initiator=%s transition=%s status=%s",
		ws.ID, ws.OwnerName, ws.Name, ws.OwnerID, ws.LatestBuild.ID, ws.LatestBuild.InitiatorID, ws.LatestBuild.InitiatorUsername, ws.LatestBuild.Transition, ws.LatestBuild.Status)
	r.check(ws.OwnerID == bobID, "step4: workspace owner_id == bob (%s)", bobID)
	r.check(ws.LatestBuild.InitiatorID == bobID, "step4: latest_build.initiator_id == bob (%s)", bobID)
	chat, err = admin.GetChat(ctx, chat.ID)
	if err != nil {
		return err
	}
	r.ev("step4: GET /chats/{id} owner_id=%s workspace_id=%s agent_id=%s status=%s", chat.OwnerID, uuidStr(chat.WorkspaceID), uuidStr(chat.AgentID), chat.Status)
	r.check(chat.WorkspaceID != nil && *chat.WorkspaceID == ws.ID, "step4: chat workspace_id == bob-from-chat (%s)", ws.ID)
	r.check(chat.AgentID != nil, "step4: chat agent_id set after create turn (got %s)", uuidStr(chat.AgentID))
	r.check(chat.OwnerID == aliceID, "step4: chat owner still alice")

	db, err := openDevDB()
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
SELECT a.id, a.user_id, u.username, a.action::text, a.resource_type::text, a.resource_target, a.status_code, a.time
FROM audit_logs a LEFT JOIN users u ON u.id = a.user_id
WHERE a.resource_type IN ('workspace', 'workspace_build') AND a.resource_id = $1
ORDER BY a.time ASC`, ws.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	auditRows := 0
	auditByBob := 0
	for rows.Next() {
		var id, userID uuid.UUID
		var username, action, resourceType, target string
		var status int
		var at time.Time
		if err := rows.Scan(&id, &userID, &username, &action, &resourceType, &target, &status, &at); err != nil {
			return err
		}
		auditRows++
		if userID == bobID {
			auditByBob++
		}
		r.ev("step4: audit_logs row id=%s user_id=%s (%s) action=%s resource_type=%s target=%s status=%d time=%s", id, userID, username, action, resourceType, target, status, at.Format(time.RFC3339))
	}
	r.check(auditRows > 0, "step4: audit_logs has %d row(s) for workspace %s", auditRows, ws.ID)
	r.check(auditRows > 0 && auditByBob == auditRows, "step4: every audit_logs row for the creation has user_id == bob (%d/%d)", auditByBob, auditRows)

	return s9bActorChecks(ctx, st, r, admin, chat, ws)
}

// s9bActorChecks runs S9B steps 5-7 against an existing chat and workspace.
func s9bActorChecks(ctx context.Context, st *state, r *reporter, admin *codersdk.Client, chat codersdk.Chat, ws codersdk.Workspace) error {
	alice := st.tokenClient("alice")
	bob := st.tokenClient("bob")
	aliceID := st.Users["alice"].ID
	bobID := st.Users["bob"].ID

	// Step 5: bob runs hostname in his own workspace. gpt-4.1-mini sometimes
	// invents a workdir that does not exist in the docker image, which makes
	// the agent's process start fail; retry once with an explicit prompt.
	prompts := []string{
		executePrompt,
		"Use the execute tool to run `hostname` in the workspace. Pass only the command argument and no workdir. Reply with the output.",
	}
	var t turnResult
	var err error
	for attempt, prompt := range prompts {
		t, err = postAndWait(ctx, bob, chat.ID, prompt, 3*time.Minute)
		if err != nil {
			r.failf("step5: bob execute post/wait: %v", err)
			return nil
		}
		r.ev("step5: bob execute turn (attempt %d): %s", attempt+1, describeTurn(t))
		retry := false
		for _, res := range t.toolResults() {
			if res.ToolName == "execute" && strings.Contains(resultText(res), `"success":false`) && !strings.Contains(resultText(res), "access denied") {
				retry = true
			}
		}
		if !retry {
			break
		}
	}
	var execFound bool
	for _, res := range t.toolResults() {
		if res.ToolName != "execute" {
			continue
		}
		execFound = true
		txt := resultText(res)
		r.check(!res.IsError, "step5: execute tool-result is_error=false (got %v)", res.IsError)
		r.check(!strings.Contains(txt, "access denied"), "step5: execute tool-result has no denial text")
		r.check(strings.Contains(txt, s9bWorkspaceName), "step5: execute output contains %q", s9bWorkspaceName)
	}
	r.check(execFound, "step5: an execute tool-result was recorded for bob")

	// Step 6: alice (chat owner, no ACL on bob's workspace) is denied.
	t, err = postAndWait(ctx, alice, chat.ID, executePrompt, 3*time.Minute)
	if err != nil {
		r.failf("step6: alice execute post/wait: %v", err)
		return nil
	}
	r.ev("step6: alice execute turn: %s", describeTurn(t))
	want := fmt.Sprintf("workspace access denied: user alice does not have access to workspace bob/%s", s9bWorkspaceName)
	execFound = false
	for _, res := range t.toolResults() {
		if res.ToolName != "execute" {
			continue
		}
		execFound = true
		r.check(res.IsError, "step6: execute tool-result is_error=true (got %v)", res.IsError)
		r.check(strings.Contains(resultText(res), want), "step6: execute tool-result contains %q", want)
	}
	r.check(execFound, "step6: an execute tool-result was recorded for alice")
	lines := grepLog(developLog, "actor_id="+aliceID.String())
	if len(lines) > 0 {
		r.ev("step6: log: %s", truncate(lines[len(lines)-1], 400))
	}

	// Step 7 (optional): stop_workspace as alice must fail, as bob must succeed.
	stopPrompt := "Use the stop_workspace tool to stop the workspace and reply with the result."
	stopResult := func(name string, c *codersdk.Client) (codersdk.ChatMessagePart, bool, error) {
		for attempt := 1; attempt <= 2; attempt++ {
			t, err := postAndWait(ctx, c, chat.ID, stopPrompt, 5*time.Minute)
			if err != nil {
				return codersdk.ChatMessagePart{}, false, err
			}
			r.ev("step7: %s stop turn (attempt %d): %s", name, attempt, describeTurn(t))
			for _, res := range t.toolResults() {
				if res.ToolName == "stop_workspace" {
					return res, true, nil
				}
			}
		}
		return codersdk.ChatMessagePart{}, false, nil
	}
	res, ok, err := stopResult("alice", alice)
	if err != nil {
		r.failf("step7: alice stop post/wait: %v", err)
		return nil
	}
	if !ok {
		r.skip("step7: model did not call stop_workspace for alice after one retry")
		return nil
	}
	txt := resultText(res)
	denied := res.IsError || strings.Contains(txt, `"error"`) || strings.Contains(strings.ToLower(txt), "not found") || strings.Contains(strings.ToLower(txt), "denied") || strings.Contains(strings.ToLower(txt), "forbidden")
	r.check(denied && !strings.Contains(txt, `"stopped":true`), "step7: alice stop_workspace tool-result is a permission error (is_error=%v)", res.IsError)
	wsAfter, err := admin.Workspace(ctx, ws.ID)
	if err == nil {
		r.check(wsAfter.LatestBuild.Transition == codersdk.WorkspaceTransitionStart, "step7: workspace still on start build after alice's attempt (transition=%s)", wsAfter.LatestBuild.Transition)
	}
	res, ok, err = stopResult("bob", bob)
	if err != nil {
		r.failf("step7: bob stop post/wait: %v", err)
		return nil
	}
	if !ok {
		r.skip("step7: model did not call stop_workspace for bob after one retry")
		return nil
	}
	txt = resultText(res)
	r.check(!res.IsError && strings.Contains(txt, `"stopped":true`), "step7: bob stop_workspace tool-result succeeds (stopped=true)")
	wsAfter, err = admin.Workspace(ctx, ws.ID)
	if err == nil {
		r.ev("step7: workspace latest_build id=%s transition=%s initiator_id=%s status=%s", wsAfter.LatestBuild.ID, wsAfter.LatestBuild.Transition, wsAfter.LatestBuild.InitiatorID, wsAfter.LatestBuild.Status)
		r.check(wsAfter.LatestBuild.Transition == codersdk.WorkspaceTransitionStop && wsAfter.LatestBuild.InitiatorID == bobID, "step7: stop build initiated by bob")
	}
	return nil
}

// ---------------------------------------------------------------- helpers

func grepLog(path, needle string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for sc.Scan() {
		if strings.Contains(sc.Text(), needle) {
			out = append(out, sc.Text())
		}
	}
	return out
}
