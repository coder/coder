package mcpclient

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

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

func httpClientWithHeaders(headers map[string]string, signingSecret string) *http.Client {
	base := http.DefaultTransport
	if isolated := mcpHTTPClient(); isolated != nil {
		base = isolated.Transport
	}
	if len(headers) == 0 && signingSecret == "" {
		return &http.Client{Transport: base}
	}
	return &http.Client{Transport: &signingRoundTripper{
		base:          base,
		headers:       headers,
		signingSecret: signingSecret,
		now:           time.Now,
	}}
}

type signingRoundTripper struct {
	base          http.RoundTripper
	headers       map[string]string
	signingSecret string
	now           func() time.Time
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
		timestamp := strconv.FormatInt(s.now().Unix(), 10)
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

	if req.GetBody != nil {
		bodyReader, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		defer bodyReader.Close()
		body, err := io.ReadAll(bodyReader)
		if err != nil {
			return nil, err
		}
		getBody := func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
		clone.Body, err = getBody()
		if err != nil {
			return nil, err
		}
		clone.GetBody = getBody
		return body, nil
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	getBody := func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = getBody
	cloneBody, err := getBody()
	if err != nil {
		return nil, err
	}
	clone.Body = cloneBody
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

// mcpHTTPClient returns an isolated *http.Client when running
// inside tests, or nil for production. During tests,
// httptest.Server.Close() calls
// http.DefaultTransport.CloseIdleConnections(), which disrupts
// any MCP client sharing that transport. When DefaultTransport
// is a *http.Transport it is cloned; otherwise a minimal
// transport with ProxyFromEnvironment is created as a fallback.
func mcpHTTPClient() *http.Client {
	if flag.Lookup("test.v") == nil {
		return nil
	}
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		return &http.Client{Transport: dt.Clone()}
	}
	return &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}}
}
