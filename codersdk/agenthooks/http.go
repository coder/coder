package agenthooks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"golang.org/x/xerrors"
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

// NewHTTPHandler returns an HTTP handler that verifies and routes lifecycle
// hook requests.
func NewHTTPHandler(secret []byte, hooks Hooks) http.Handler {
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
		if err := verifyBody(r, body, claims, request); err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
		response, err := dispatch(r.Context(), hooks, request)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		rw.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(rw).Encode(response); err != nil {
			return
		}
	})
}

func verifyBody(r *http.Request, body []byte, claims Claims, request Request) error {
	digest := sha256.Sum256(body)
	if claims.BodySHA256 != hex.EncodeToString(digest[:]) {
		return xerrors.New("request body does not match body_sha256 claim")
	}
	if claims.Audience != requestAudience(r) {
		return xerrors.New("request URL does not match audience claim")
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

func requestAudience(r *http.Request) string {
	url := *r.URL
	if url.Scheme == "" {
		url.Scheme = "http"
		if r.TLS != nil {
			url.Scheme = "https"
		}
	}
	if url.Host == "" {
		url.Host = r.Host
	}
	if url.Path == "/" && url.RawPath == "" {
		url.Path = ""
	}
	return url.String()
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
