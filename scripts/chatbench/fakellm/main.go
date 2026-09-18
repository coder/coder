// Command fakellm is an OpenAI-compatible chat completions server used by
// the chat rendering benchmark. It streams deterministic markdown at a
// configurable token rate so the browser sees a realistic, long-lived
// streaming turn without a real model provider.
//
// The behavior of a turn is chosen by a directive in the last user message:
//
//	BENCH:seed <n>          reply promptly with a medium markdown message (n varies content)
//	BENCH:stream <seconds>  stream markdown for <seconds> at --rate tokens/second
//	(anything else)         reply promptly with a short message
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type request struct {
	Model    string    `json:"model"`
	Stream   bool      `json:"stream"`
	Messages []message `json:"messages"`
}

type message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func (m message) text() string {
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(m.Content, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			_, _ = b.WriteString(p.Text)
		}
		return b.String()
	}
	return ""
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18081", "listen address")
	rate := flag.Float64("rate", 40, "streamed tokens per second in BENCH:stream mode")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat/completions", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var req request
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		lastUser := ""
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == "user" {
				lastUser = req.Messages[i].text()
				break
			}
		}
		text, delay := plan(lastUser, *rate)
		log.Printf("turn model=%s stream=%v msgs=%d directive=%q chars=%d delay=%s", req.Model, req.Stream, len(req.Messages), directive(lastUser), len(text), delay)
		if !req.Stream {
			writeJSON(w, completion(req.Model, text))
			return
		}
		streamText(w, r, req.Model, text, delay)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	log.Printf("fakellm listening on %s", *addr)
	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

func directive(s string) string {
	i := strings.Index(s, "BENCH:")
	if i < 0 {
		return ""
	}
	f := strings.Fields(s[i:])
	if len(f) >= 2 {
		return f[0] + " " + f[1]
	}
	return f[0]
}

// plan returns the reply text and the per-token delay for the given
// user message.
func plan(user string, rate float64) (string, time.Duration) {
	f := strings.Fields(directive(user))
	if len(f) < 2 {
		return "Acknowledged.", 0
	}
	switch f[0] {
	case "BENCH:seed":
		n, _ := strconv.Atoi(f[1])
		return markdown(int64(n), 6+n%5), 0
	case "BENCH:stream":
		secs, _ := strconv.Atoi(f[1])
		perTok := time.Duration(float64(time.Second) / rate)
		tokens := int(float64(secs) * rate)
		var b strings.Builder
		seed := int64(7)
		for countTokens(b.String()) < tokens {
			_, _ = b.WriteString(markdown(seed, 8))
			_, _ = b.WriteString("\n\n")
			seed++
		}
		return b.String(), perTok
	}
	return "Acknowledged.", 0
}

func completion(model, text string) map[string]any {
	return map[string]any{
		"id":      "chatcmpl-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": text},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": countTokens(text), "total_tokens": 10 + countTokens(text)},
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// streamText emits SSE chunks, one token per chunk, pausing delay
// between tokens.
func streamText(w http.ResponseWriter, r *http.Request, model, text string, delay time.Duration) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	id := "chatcmpl-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	created := time.Now().Unix()
	write := func(delta map[string]any, finish any) bool {
		chunk := map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
		}
		b, _ := json.Marshal(chunk)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !write(map[string]any{"role": "assistant", "content": ""}, nil) {
		return
	}
	var ticker *time.Ticker
	if delay > 0 {
		ticker = time.NewTicker(delay)
		defer ticker.Stop()
	}
	sent := 0
	for _, tok := range tokenize(text) {
		if ticker != nil {
			select {
			case <-r.Context().Done():
				log.Printf("client went away after %d tokens", sent)
				return
			case <-ticker.C:
			}
		}
		if !write(map[string]any{"content": tok}, nil) {
			return
		}
		sent++
	}
	write(map[string]any{}, "stop")
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
	log.Printf("stream complete: %d tokens", sent)
}

// tokenize splits text into word-sized chunks that keep their trailing
// whitespace so concatenation reproduces the input exactly.
func tokenize(s string) []string {
	var out []string
	start := 0
	inSpace := false
	for i, r := range s {
		isSpace := r == ' ' || r == '\n' || r == '\t'
		if inSpace && !isSpace {
			out = append(out, s[start:i])
			start = i
		}
		inSpace = isSpace
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func countTokens(s string) int { return len(tokenize(s)) }

var words = strings.Fields(`the build pipeline reads every workspace template and resolves provisioner
jobs before scheduling them across the available daemons while the coordinator tracks tailnet peers
and agents report their lifecycle state through the control plane so that dashboards stay accurate
even when connections flap between regions this keeps the audit log consistent and lets operators
reason about failures without guessing which component dropped the request first retries are
bounded and jittered to avoid thundering herds and the database queries stay indexed on the hot
columns such as organization id and updated at timestamps`)

var codeSnippets = []string{
	"```bash\nmake build-slim\n./build/coder-slim server --http-address 127.0.0.1:3000\n```",
	"```go\nfunc (s *Server) handle(ctx context.Context, req Request) error {\n\tif err := s.validate(req); err != nil {\n\t\treturn xerrors.Errorf(\"validate: %w\", err)\n\t}\n\treturn s.store.Save(ctx, req)\n}\n```",
	"```ts\nconst rows = messages.filter((m) => m.role !== \"tool\");\nfor (const row of rows) {\n\trender(row);\n}\n```",
	"```sql\nSELECT id, updated_at FROM chats WHERE organization_id = $1 ORDER BY updated_at DESC LIMIT 50;\n```",
}

// markdown renders deterministic prose-heavy markdown with headings,
// lists, emphasis, inline code and an occasional short fenced block.
func markdown(seed int64, paragraphs int) string {
	h := fnv.New64a()
	_, _ = fmt.Fprint(h, seed)
	rng := rand.New(rand.NewSource(int64(h.Sum64()))) //nolint:gosec // deterministic benchmark content
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "## %s\n\n", title(rng, 3+rng.Intn(3)))
	for p := 0; p < paragraphs; p++ {
		switch rng.Intn(7) {
		case 0:
			for i := 0; i < 3+rng.Intn(3); i++ {
				_, _ = fmt.Fprintf(&b, "- **%s**: %s\n", title(rng, 2), sentence(rng, 8+rng.Intn(10)))
			}
			_, _ = b.WriteString("\n")
		case 1:
			for i := 0; i < 3+rng.Intn(2); i++ {
				_, _ = fmt.Fprintf(&b, "%d. %s\n", i+1, sentence(rng, 8+rng.Intn(8)))
			}
			_, _ = b.WriteString("\n")
		case 2:
			_, _ = b.WriteString(codeSnippets[rng.Intn(len(codeSnippets))])
			_, _ = b.WriteString("\n\n")
		case 3:
			_, _ = fmt.Fprintf(&b, "### %s\n\n", title(rng, 2+rng.Intn(3)))
			_, _ = b.WriteString(paragraph(rng, 3+rng.Intn(3)))
			_, _ = b.WriteString("\n\n")
		default:
			_, _ = b.WriteString(paragraph(rng, 3+rng.Intn(4)))
			_, _ = b.WriteString("\n\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func title(rng *rand.Rand, n int) string {
	ws := make([]string, n)
	for i := range ws {
		w := words[rng.Intn(len(words))]
		ws[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(ws, " ")
}

func paragraph(rng *rand.Rand, sentences int) string {
	parts := make([]string, sentences)
	for i := range parts {
		parts[i] = sentence(rng, 10+rng.Intn(14))
	}
	return strings.Join(parts, " ")
}

func sentence(rng *rand.Rand, n int) string {
	ws := make([]string, n)
	for i := range ws {
		w := words[rng.Intn(len(words))]
		switch rng.Intn(18) {
		case 0:
			w = "`" + w + "`"
		case 1:
			w = "**" + w + "**"
		case 2:
			w = "_" + w + "_"
		}
		ws[i] = w
	}
	s := strings.Join(ws, " ")
	return strings.ToUpper(s[:1]) + s[1:] + "."
}
