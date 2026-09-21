package mcpclient

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/safedial"
)

// responseHeaderTimeout bounds how long a server may take to send
// response headers once a request is written. It matches
// toolCallTimeout rather than connectTimeout because the same
// client serves tool-call POSTs, and a JSON-response MCP server
// sends no headers until the tool finishes, so a lower value would
// kill legitimate slow tools that fit the tool-call budget.
// Long-lived SSE streams are unaffected; only their headers must
// arrive within this window. http.Client.Timeout is deliberately
// unset because it would cap the stream body too.
const responseHeaderTimeout = toolCallTimeout

// NewHTTPClient builds the SSRF-guarded, timeout-bounded HTTP
// client for MCP traffic. MCP destinations are configured by org
// admins rather than the deployment operator, so every connection
// goes through safedial's address policy. The guarded transport
// keeps the base client's settings but always bounds response
// headers: without a bound, a server that accepts a request and
// never answers holds the connection until the enclosing context
// expires. Dial establishment is bounded by the guarded dialer.
func NewHTTPClient(base *http.Client, opts ...safedial.Option) *http.Client {
	if base == nil {
		// Not safedial's nil default: that carries a whole-client
		// timeout, which would cap SSE stream bodies that must stay
		// open for the life of a session.
		base = &http.Client{}
	}
	client := safedial.NewHTTPClient(base, opts...)
	if tr, ok := client.Transport.(*http.Transport); ok {
		// The guarded transport is a private clone, so setting the
		// bound here cannot race with or mutate the caller's base.
		tr.ResponseHeaderTimeout = responseHeaderTimeout
	}
	return client
}

const (
	// HeaderCoderSignatureTimestamp contains the Unix timestamp used to sign
	// the request.
	HeaderCoderSignatureTimestamp = "X-Coder-Signature-Timestamp"
	// HeaderCoderSignature contains the versioned HMAC-SHA256 request
	// signature.
	HeaderCoderSignature = "X-Coder-Signature"

	headerCoderOwnerID     = "X-Coder-Owner-Id"
	headerCoderChatID      = "X-Coder-Chat-Id"
	headerCoderSubchatID   = "X-Coder-Subchat-Id"
	headerCoderWorkspaceID = "X-Coder-Workspace-Id"
)

func httpClientWithHeaders(base *http.Client, headers map[string]string, signingSecret string) *http.Client {
	if base == nil || base.Transport == nil {
		base = NewHTTPClient(base)
	}
	client := *base
	client.CheckRedirect = safedial.CheckSameOriginRedirect
	if len(headers) == 0 && signingSecret == "" {
		return &client
	}
	client.Transport = &signingRoundTripper{
		base:          base.Transport,
		headers:       headers,
		signingSecret: signingSecret,
	}
	return &client
}

type signingRoundTripper struct {
	base          http.RoundTripper
	headers       map[string]string
	signingSecret string
}

func (s *signingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for k, v := range s.headers {
		clone.Header.Set(k, v)
	}
	if s.signingSecret != "" {
		body, err := bufferRequestBody(req, clone)
		if err != nil {
			return nil, xerrors.Errorf("buffer MCP request body: %w", err)
		}
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		clone.Header.Set(HeaderCoderSignatureTimestamp, timestamp)
		clone.Header.Set(HeaderCoderSignature, signMCPRequest(
			s.signingSecret,
			mcpSignatureCanonical(clone, timestamp, body),
		))
	}
	return s.base.RoundTrip(clone)
}

func bufferRequestBody(req, clone *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}

	reader := req.Body
	if req.GetBody != nil {
		var err error
		reader, err = req.GetBody()
		if err != nil {
			return nil, err
		}
	}
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		return nil, err
	}
	getBody := func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	if req.GetBody == nil {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.GetBody = getBody
	}
	clone.Body = io.NopCloser(bytes.NewReader(body))
	clone.GetBody = getBody
	return body, nil
}

func mcpSignatureCanonical(req *http.Request, timestamp string, body []byte) string {
	bodyHash := sha256.Sum256(body)
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	return strings.Join([]string{
		"v1",
		timestamp,
		strings.ToUpper(method),
		req.URL.RequestURI(),
		hex.EncodeToString(bodyHash[:]),
		"owner=" + req.Header.Get(headerCoderOwnerID),
		"chat=" + req.Header.Get(headerCoderChatID),
		"subchat=" + req.Header.Get(headerCoderSubchatID),
		"workspace=" + req.Header.Get(headerCoderWorkspaceID),
	}, "\n")
}

func signMCPRequest(signingSecret, canonical string) string {
	mac := hmac.New(sha256.New, []byte(signingSecret))
	_, _ = mac.Write([]byte(canonical))
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}
