package agenthooks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"golang.org/x/xerrors"
)

// MaxRequestBodyBytes limits memory used to verify hook requests.
const MaxRequestBodyBytes int64 = 10 << 20 // 10 MiB

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

// HandlerOption configures NewHTTPHandler.
type HandlerOption func(*handlerOptions)

type handlerOptions struct {
	expectedIssuer string
}

// WithExpectedIssuer requires the verified iss claim to match issuer.
// If omitted, NewHTTPHandler accepts any non-empty issuer signed
// with the secret.
func WithExpectedIssuer(issuer string) HandlerOption {
	return func(options *handlerOptions) {
		options.expectedIssuer = issuer
	}
}

// NewHTTPHandler verifies hook POSTs, binds their claims to each request,
// and routes events to their configured callbacks. expectedAudience must be
// the URL Coder dispatches to, which is the value it signs into the aud
// claim. Deriving it from the request instead would let a caller replay a
// token minted for a different listener, because the request URL, the Host
// header, and any forwarding headers are all caller-controlled. A handler
// built with an empty audience rejects every request.
func NewHTTPHandler(secret []byte, expectedAudience string, hooks Hooks, opts ...HandlerOption) http.Handler {
	var options handlerOptions
	for _, opt := range opts {
		opt(&options)
	}
	if expectedAudience == "" {
		return http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			http.Error(rw, "hook audience is not configured", http.StatusInternalServerError)
		})
	}
	audience := canonicalAudience(expectedAudience)
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
		if options.expectedIssuer != "" && claims.Issuer != options.expectedIssuer {
			http.Error(rw, "unexpected issuer", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(rw, r.Body, MaxRequestBodyBytes)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				http.Error(rw, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(rw, "read request body", http.StatusBadRequest)
			return
		}
		var request Request
		if err := json.Unmarshal(body, &request); err != nil {
			http.Error(rw, "decode request body", http.StatusBadRequest)
			return
		}
		if err := verifyBody(body, claims, request, audience); err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
		response, err := dispatch(r.Context(), hooks, request)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		// Marshal before writing: streaming the encode would emit a 200
		// with a truncated body on failure, and the dispatcher reads an
		// empty 200 as allow, so a malformed deny would fail open.
		encoded, err := json.Marshal(response)
		if err != nil {
			http.Error(rw, "encode response", http.StatusInternalServerError)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		if _, err := rw.Write(encoded); err != nil {
			return
		}
	})
}

func verifyBody(body []byte, claims Claims, request Request, audience string) error {
	digest := sha256.Sum256(body)
	if claims.BodySHA256 != hex.EncodeToString(digest[:]) {
		return xerrors.New("request body does not match body_sha256 claim")
	}
	if canonicalAudience(claims.Audience) != audience {
		return xerrors.New("audience claim does not match the configured audience")
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
