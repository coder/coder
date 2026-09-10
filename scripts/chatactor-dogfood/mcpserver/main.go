// Command mcpserver is a tiny streamable HTTP MCP server used to dogfood
// per-actor identity forwarding from chatd. It exposes one useful tool,
// whoami, that echoes the Coder identity headers it received, plus a
// decoy tool, noop, that the Coder MCP config denies so chatd emits a
// debug log line carrying the turn actor_id on every connect.
//
// Token labels: set TOKEN_LABELS="token1=alice,token2=bob" so the server
// reports a label instead of the bearer token it received. Tokens are
// never printed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type whoamiResult struct {
	OwnerID     string `json:"owner_id"`
	ActorID     string `json:"actor_id"`
	ChatID      string `json:"chat_id"`
	WorkspaceID string `json:"workspace_id"`
	TokenLabel  string `json:"token_label"`
	HasAuth     bool   `json:"has_authorization_header"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:3999", "listen address")
	logPath := flag.String("log", "", "append JSON lines of every whoami call to this file")
	flag.Parse()

	labels := map[string]string{}
	for _, kv := range strings.Split(os.Getenv("TOKEN_LABELS"), ",") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			log.Fatalf("bad TOKEN_LABELS entry %q", kv)
		}
		labels[parts[0]] = parts[1]
	}

	var (
		mu      sync.Mutex
		logFile *os.File
	)
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			log.Fatal(err)
		}
		logFile = f
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: "dogfood-whoami", Version: "1.0.0"}, nil)
	srv.AddTool(&mcp.Tool{
		Name:        "whoami",
		Description: "Returns the Coder identity headers received by the MCP server as JSON.",
		InputSchema: map[string]any{"type": "object"},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		h := req.Extra.Header
		res := whoamiResult{
			OwnerID:     h.Get("X-Coder-Owner-Id"),
			ActorID:     h.Get("X-Coder-Actor-Id"),
			ChatID:      h.Get("X-Coder-Chat-Id"),
			WorkspaceID: h.Get("X-Coder-Workspace-Id"),
		}
		if auth := h.Get("Authorization"); auth != "" {
			res.HasAuth = true
			tok := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			if label, ok := labels[tok]; ok {
				res.TokenLabel = label
			} else {
				res.TokenLabel = "unknown"
			}
		}
		b, _ := json.Marshal(res)
		log.Printf("whoami call: %s", b)
		if logFile != nil {
			mu.Lock()
			_, _ = logFile.Write(append(b, '\n'))
			mu.Unlock()
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
	})
	srv.AddTool(&mcp.Tool{
		Name:        "noop",
		Description: "Does nothing. Denied by the Coder MCP config on purpose.",
		InputSchema: map[string]any{"type": "object"},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "noop"}}}, nil
	})

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, "ok")
	})
	log.Printf("dogfood MCP server listening on http://%s/mcp (labels: %d)", *addr, len(labels))
	server := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
