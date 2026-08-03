package agenthooks

import (
	"bytes"
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
		claims, _, request, ok := verifyHookRequest(rw, r, secret, audience, options.expectedIssuer)
		if !ok {
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
		signature, err := SignResponse(secret, claims, encoded)
		if err != nil {
			http.Error(rw, "sign response", http.StatusInternalServerError)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set(SignatureHeader, signature)
		if _, err := rw.Write(encoded); err != nil {
			return
		}
	})
}

// verifyHookRequest authenticates one dispatcher request for a handler:
// POST-only, bearer token, optional issuer pin, size-capped body read, and
// the verifyBody binding of the exact bytes and their metadata (dispatch
// ID, event type, chat, audience) to the token claims. It writes the error
// response itself and reports ok=false when the request must not reach any
// hook logic.
func verifyHookRequest(rw http.ResponseWriter, r *http.Request, secret []byte, audience, expectedIssuer string) (Claims, []byte, Request, bool) {
	if r.Method != http.MethodPost {
		rw.Header().Set("Allow", http.MethodPost)
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return Claims{}, nil, Request{}, false
	}
	claims, err := Verify(r.Header.Get("Authorization"), secret)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusUnauthorized)
		return Claims{}, nil, Request{}, false
	}
	if expectedIssuer != "" && claims.Issuer != expectedIssuer {
		http.Error(rw, "unexpected issuer", http.StatusUnauthorized)
		return Claims{}, nil, Request{}, false
	}
	r.Body = http.MaxBytesReader(rw, r.Body, MaxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(rw, "request body too large", http.StatusRequestEntityTooLarge)
			return Claims{}, nil, Request{}, false
		}
		http.Error(rw, "read request body", http.StatusBadRequest)
		return Claims{}, nil, Request{}, false
	}
	var request Request
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(rw, "decode request body", http.StatusBadRequest)
		return Claims{}, nil, Request{}, false
	}
	if err := verifyBody(body, claims, request, audience); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return Claims{}, nil, Request{}, false
	}
	return claims, body, request, true
}

// SignResponses wraps a consumer's own hook handler so every 2xx response
// carries the SignatureHeader token the dispatcher requires. It
// authenticates each request exactly like NewHTTPHandler (bearer token,
// audience, and the claims' binding to the exact body bytes) before
// invoking next: signing on the token alone would let anyone who observed
// a valid request replay its token with a different body and use the
// wrapper as a signing oracle for decisions the dispatcher trusts. next
// reads the verified body. Like NewHTTPHandler, expectedAudience must be
// the URL Coder dispatches to; an empty audience rejects every request.
func SignResponses(secret []byte, expectedAudience string, next http.Handler) http.Handler {
	if expectedAudience == "" {
		return http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			http.Error(rw, "hook audience is not configured", http.StatusInternalServerError)
		})
	}
	audience := canonicalAudience(expectedAudience)
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		claims, body, _, ok := verifyHookRequest(rw, r, secret, audience, "")
		if !ok {
			return
		}
		// Hand the handler the verified bytes, not the network stream.
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		buffer := &bufferedResponse{header: make(http.Header)}
		next.ServeHTTP(buffer, r)
		buffer.commitStatus()
		responseBody := buffer.body.Bytes()
		if buffer.status >= http.StatusOK && buffer.status < http.StatusMultipleChoices {
			signature, err := SignResponse(secret, claims, responseBody)
			if err != nil {
				http.Error(rw, "sign response", http.StatusInternalServerError)
				return
			}
			buffer.committed.Set(SignatureHeader, signature)
		}
		for key, values := range buffer.committed {
			rw.Header()[key] = values
		}
		rw.WriteHeader(buffer.status)
		_, _ = rw.Write(responseBody)
	})
}

// bufferedResponse captures a handler's response so it can be signed before
// any byte reaches the network. It mirrors net/http commit semantics: the
// first WriteHeader wins, Write commits an implicit 200, later status
// changes are ignored, and the headers are snapshotted at the commit, so a
// handler that wrote an error status cannot be converted into a signed 2xx
// and header mutations after the first write cannot desynchronize the
// forwarded headers from the signed body.
type bufferedResponse struct {
	header http.Header
	// committed is the header snapshot frozen by the first effective
	// WriteHeader, exactly like net/http, which writes the headers to the
	// wire at that point. Only this snapshot is forwarded upstream;
	// mutations to header after the commit have no effect.
	committed   http.Header
	body        bytes.Buffer
	status      int
	wroteHeader bool
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) Write(p []byte) (int, error) {
	b.commitStatus()
	// net/http suppresses bodies for these statuses. Refusing the write
	// here keeps the buffer identical to the bytes the real writer sends,
	// so the signature computed over the buffer always matches the wire
	// body the dispatcher hashes.
	if !bodyAllowedForStatus(b.status) {
		return 0, http.ErrBodyNotAllowed
	}
	return b.body.Write(p)
}

// bodyAllowedForStatus mirrors net/http's rule for statuses that cannot
// carry a response body.
func bodyAllowedForStatus(status int) bool {
	switch {
	case status >= 100 && status <= 199:
		return false
	case status == http.StatusNoContent:
		return false
	case status == http.StatusNotModified:
		return false
	}
	return true
}

func (b *bufferedResponse) WriteHeader(status int) {
	if b.wroteHeader {
		return
	}
	// net/http sends informational 1xx responses (except 101, which is
	// final) immediately without committing, and still accepts the
	// handler's real final status afterwards. Interim responses carry
	// nothing the dispatcher uses, so the buffer drops them instead of
	// forwarding, and keeps accepting the first non-1xx status so the
	// signed response is the final one the handler produced.
	if status >= 100 && status <= 199 && status != http.StatusSwitchingProtocols {
		return
	}
	b.status = status
	b.wroteHeader = true
	b.committed = b.header.Clone()
}

// commitStatus locks in the implicit 200 a real ResponseWriter would send
// on the first Write (or at end of a handler that never wrote anything).
func (b *bufferedResponse) commitStatus() {
	if !b.wroteHeader {
		b.WriteHeader(http.StatusOK)
	}
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
