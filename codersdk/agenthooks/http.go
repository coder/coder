package agenthooks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

const (
	// maxReplayRetention caps how long a dispatch ID is remembered. Tokens
	// minted by coderd expire well within this window, so entries never
	// outlive the token they deduplicate.
	maxReplayRetention = 10 * time.Minute
	// maxReplayEntries bounds replay-cache memory. Only requests bearing a
	// valid signature reach the cache, so the cap is only reachable by the
	// deployment itself (or a compromised secret).
	maxReplayEntries = 8192
)

// Hooks lets a consumer implement only the lifecycle events it uses.
type Hooks struct {
	SessionStart     func(context.Context, Meta, SessionStartData) (Response, error)
	UserPromptSubmit func(context.Context, Meta, UserPromptSubmitData) (Response, error)
	PreToolUse       func(context.Context, Meta, PreToolUseData) (Response, error)
	PostToolUse      func(context.Context, Meta, PostToolUseData) (Response, error)
	PreCompact       func(context.Context, Meta, PreCompactData) (Response, error)
	PostCompact      func(context.Context, Meta, PostCompactData) (Response, error)
	Stop             func(context.Context, Meta, StopData) (Response, error)
}

// NewHTTPHandler verifies and dispatches lifecycle hook requests.
//
// audience must be the exact hook URL configured on the deployment
// (CODER_CHAT_HOOK_URL). Tokens whose aud claim does not match it are
// rejected; the expected audience is never derived from request data such
// as Host or X-Forwarded-* headers, so a proxy or client cannot influence
// the comparison.
//
// The handler also deduplicates dispatch IDs (jti): a replayed request is
// answered with the recorded response of its first delivery instead of
// re-invoking hooks, so replays cannot duplicate consumer side effects
// while coderd's connection-error retry (which reuses the dispatch ID)
// still receives the original decision. The cache is per-process;
// consumers running multiple replicas behind one URL need shared
// deduplication for the same guarantee.
func NewHTTPHandler(secret []byte, audience string, hooks Hooks) (http.Handler, error) {
	parsed, err := url.Parse(audience)
	if err != nil {
		return nil, xerrors.Errorf("parse audience: %w", err)
	}
	if audience == "" || !parsed.IsAbs() || parsed.Host == "" {
		return nil, xerrors.Errorf("audience must be an absolute URL, got %q", audience)
	}
	expectedAudience := canonicalAudience(audience)
	replays := &replayCache{entries: make(map[uuid.UUID]*replayEntry)}

	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			rw.Header().Set("Allow", http.MethodPost)
			http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		claims, err := Verify(r.Header.Get("Authorization"), secret)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(rw, "read request body", http.StatusBadRequest)
			return
		}
		var request Request
		if err := json.Unmarshal(body, &request); err != nil {
			http.Error(rw, "decode request body", http.StatusBadRequest)
			return
		}
		if err := verifyBody(body, claims, request, expectedAudience); err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}

		entry, isNew, err := replays.begin(claims.JTI, time.Unix(claims.Expires, 0))
		if err != nil {
			http.Error(rw, err.Error(), http.StatusServiceUnavailable)
			return
		}
		if !isNew {
			select {
			case <-entry.done:
				entry.write(rw)
			case <-r.Context().Done():
			}
			return
		}
		defer close(entry.done)

		response, err := dispatch(r.Context(), hooks, request)
		if err != nil {
			entry.status = http.StatusInternalServerError
			entry.body = []byte(err.Error() + "\n")
			entry.contentType = "text/plain; charset=utf-8"
			entry.write(rw)
			return
		}
		encoded, err := json.Marshal(response)
		if err != nil {
			entry.status = http.StatusInternalServerError
			entry.body = []byte("encode response\n")
			entry.contentType = "text/plain; charset=utf-8"
			entry.write(rw)
			return
		}
		encoded = append(encoded, '\n')
		entry.status = http.StatusOK
		entry.body = encoded
		entry.contentType = "application/json"
		entry.write(rw)
	}), nil
}

// replayEntry records the outcome of the first delivery of a dispatch ID.
// done is closed once status/body/contentType are final; replayed requests
// wait on it and then serve the recorded response.
type replayEntry struct {
	done        chan struct{}
	expires     time.Time
	status      int
	body        []byte
	contentType string
}

func (e *replayEntry) write(rw http.ResponseWriter) {
	rw.Header().Set("Content-Type", e.contentType)
	rw.WriteHeader(e.status)
	_, _ = rw.Write(e.body)
}

type replayCache struct {
	mu      sync.Mutex
	entries map[uuid.UUID]*replayEntry
}

// begin claims a dispatch ID. It returns the existing entry when the ID has
// been seen (isNew false), or a new in-flight entry the caller must complete
// and close (isNew true).
func (c *replayCache) begin(jti uuid.UUID, expires time.Time) (entry *replayEntry, isNew bool, err error) {
	now := time.Now()
	if remember := now.Add(maxReplayRetention); expires.After(remember) {
		expires = remember
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for id, existing := range c.entries {
		if now.After(existing.expires) && isDone(existing.done) {
			delete(c.entries, id)
		}
	}
	if existing, ok := c.entries[jti]; ok {
		return existing, false, nil
	}
	if len(c.entries) >= maxReplayEntries {
		return nil, false, xerrors.New("replay cache is full")
	}
	entry = &replayEntry{done: make(chan struct{}), expires: expires}
	c.entries[jti] = entry
	return entry, true, nil
}

func isDone(done chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func verifyBody(body []byte, claims Claims, request Request, expectedAudience string) error {
	digest := sha256.Sum256(body)
	if claims.BodySHA256 != hex.EncodeToString(digest[:]) {
		return xerrors.New("request body does not match body_sha256 claim")
	}
	if canonicalAudience(claims.Audience) != expectedAudience {
		return xerrors.New("audience claim does not match the configured hook URL")
	}
	if request.Meta.SchemaVersion != SchemaVersion {
		return xerrors.New("unsupported schema version")
	}
	if request.Meta.DispatchID != claims.JTI {
		return xerrors.New("dispatch ID does not match JWT ID")
	}
	if request.Type != claims.Type {
		return xerrors.New("request type does not match type claim")
	}
	chatID, err := claims.ChatID()
	if err != nil {
		return err
	}
	if request.Meta.ChatID != chatID {
		return xerrors.New("chat ID does not match subject claim")
	}
	return nil
}

// canonicalAudience treats root URLs with and without a trailing slash as equivalent.
func canonicalAudience(audience string) string {
	parsed, err := url.Parse(audience)
	if err != nil {
		return audience
	}
	if parsed.Path == "/" && parsed.RawPath == "" {
		parsed.Path = ""
	}
	return parsed.String()
}

func dispatch(ctx context.Context, hooks Hooks, request Request) (Response, error) {
	switch request.Type {
	case EventSessionStart:
		return dispatchHook(ctx, request, hooks.SessionStart)
	case EventUserPromptSubmit:
		return dispatchHook(ctx, request, hooks.UserPromptSubmit)
	case EventPreToolUse:
		return dispatchHook(ctx, request, hooks.PreToolUse)
	case EventPostToolUse:
		return dispatchHook(ctx, request, hooks.PostToolUse)
	case EventPreCompact:
		return dispatchHook(ctx, request, hooks.PreCompact)
	case EventPostCompact:
		return dispatchHook(ctx, request, hooks.PostCompact)
	case EventStop:
		return dispatchHook(ctx, request, hooks.Stop)
	default:
		return Response{}, xerrors.Errorf("unknown event type %q", request.Type)
	}
}

func dispatchHook[T any](ctx context.Context, request Request, hook func(context.Context, Meta, T) (Response, error)) (Response, error) {
	if hook == nil {
		return Response{}, nil
	}
	var data T
	if err := json.Unmarshal(request.Data, &data); err != nil {
		return Response{}, xerrors.Errorf("decode %q event data: %w", request.Type, err)
	}
	return hook(ctx, request.Meta, data)
}
