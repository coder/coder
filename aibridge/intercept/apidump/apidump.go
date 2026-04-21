package apidump

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/tidwall/pretty"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/aibridge/utils"
	"github.com/coder/quartz"
)

const (
	// SuffixRequest is the file suffix for request dump files.
	SuffixRequest = ".req.txt"
	// SuffixResponse is the file suffix for response dump files.
	SuffixResponse = ".resp.txt"
	// SuffixError is the file suffix for error dump files written when a request fails.
	SuffixError = ".req_error.txt"
)

// MiddlewareNext is the function to call the next middleware or the actual request.
type MiddlewareNext = func(*http.Request) (*http.Response, error)

// Middleware is an HTTP middleware function compatible with SDK WithMiddleware options.
type Middleware = func(*http.Request, MiddlewareNext) (*http.Response, error)

// NewBridgeMiddleware returns a middleware function that dumps requests and responses to files.
// If baseDir is empty, returns nil (no middleware).
func NewBridgeMiddleware(baseDir string, provider string, model string, interceptionID uuid.UUID, logger slog.Logger, clk quartz.Clock) Middleware {
	if baseDir == "" {
		return nil
	}

	d := &dumper{
		dumpPath: interceptDumpPath(baseDir, provider, model, interceptionID, clk),
		logger:   logger,
	}

	return func(req *http.Request, next MiddlewareNext) (*http.Response, error) {
		if err := d.dumpRequest(req); err != nil {
			logger.Named("apidump").Warn(req.Context(), "failed to dump request", slog.Error(err))
		}

		resp, err := next(req)
		if err != nil {
			if dumpErr := d.dumpError(err); dumpErr != nil {
				logger.Named("apidump").Warn(req.Context(), "failed to dump request error", slog.Error(dumpErr))
			}
			return resp, err
		}

		if err := d.dumpResponse(resp); err != nil {
			logger.Named("apidump").Warn(req.Context(), "failed to dump response", slog.Error(err))
		}

		return resp, nil
	}
}

type dumper struct {
	dumpPath string
	logger   slog.Logger
}

func (d *dumper) dumpRequest(req *http.Request) error {
	dumpPath := d.dumpPath + SuffixRequest
	if err := os.MkdirAll(filepath.Dir(dumpPath), 0o755); err != nil {
		return xerrors.Errorf("create dump dir: %w", err)
	}

	// Read and restore body
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return xerrors.Errorf("read request body: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	prettyBody := prettyPrintJSON(bodyBytes)

	// Build raw HTTP request format
	var buf bytes.Buffer
	_, err := fmt.Fprintf(&buf, "%s %s %s\r\n", req.Method, req.URL.RequestURI(), req.Proto)
	if err != nil {
		return xerrors.Errorf("write request uri: %w", err)
	}
	err = d.writeRedactedHeaders(&buf, req.Header, sensitiveRequestHeaders, map[string]string{
		"Content-Length": fmt.Sprintf("%d", len(prettyBody)),
	})
	if err != nil {
		return xerrors.Errorf("write request headers: %w", err)
	}

	_, err = fmt.Fprintf(&buf, "\r\n")
	if err != nil {
		return xerrors.Errorf("write request header terminator: %w", err)
	}
	// bytes.Buffer writes to in-memory storage and never return errors.
	_, _ = buf.Write(prettyBody)
	_ = buf.WriteByte('\n')

	return os.WriteFile(dumpPath, buf.Bytes(), 0o644) //nolint:gosec // https://github.com/coder/aibridge/pull/256#discussion_r3072143983
}

func (d *dumper) dumpError(reqErr error) error {
	dumpPath := d.dumpPath + SuffixError
	if err := os.MkdirAll(filepath.Dir(dumpPath), 0o755); err != nil {
		return xerrors.Errorf("create dump dir: %w", err)
	}
	return os.WriteFile(dumpPath, []byte(reqErr.Error()+"\n"), 0o644) //nolint:gosec // same rationale as other dump files
}

func (d *dumper) dumpResponse(resp *http.Response) error {
	dumpPath := d.dumpPath + SuffixResponse

	// Build raw HTTP response headers
	var headerBuf bytes.Buffer
	_, err := fmt.Fprintf(&headerBuf, "%s %s\r\n", resp.Proto, resp.Status)
	if err != nil {
		return xerrors.Errorf("write response status: %w", err)
	}
	err = d.writeRedactedHeaders(&headerBuf, resp.Header, sensitiveResponseHeaders, nil)
	if err != nil {
		return xerrors.Errorf("write response headers: %w", err)
	}
	_, err = fmt.Fprintf(&headerBuf, "\r\n")
	if err != nil {
		return xerrors.Errorf("write response header terminator: %w", err)
	}

	if resp.Body == nil {
		// No body, just write headers
		return os.WriteFile(dumpPath, headerBuf.Bytes(), 0o644) //nolint:gosec // https://github.com/coder/aibridge/pull/256#discussion_r3072143983
	}

	// Wrap the response body to capture it as it streams
	resp.Body = &streamingBodyDumper{
		body:       resp.Body,
		dumpPath:   dumpPath,
		headerData: headerBuf.Bytes(),
		logger: func(err error) {
			d.logger.Named("apidump").Warn(context.Background(), "failed to initialize response dump", slog.Error(err))
		},
	}

	return nil
}

// writeRedactedHeaders writes HTTP headers in wire format (Key: Value\r\n) to w,
// redacting sensitive values and applying any overrides. Headers are sorted by key
// for deterministic output.
// `sensitive` and `overrides` must both supply keys in canonicalized form.
// See [textproto.MIMEHeader].
func (*dumper) writeRedactedHeaders(w io.Writer, headers http.Header, sensitive map[string]struct{}, overrides map[string]string) error {
	// Collect all header keys including overrides.
	headerKeys := make([]string, 0, len(headers)+len(overrides))
	seen := make(map[string]struct{}, len(headers)+len(overrides))
	for key := range headers {
		headerKeys = append(headerKeys, key)
		seen[key] = struct{}{}
	}
	// Add override keys that don't exist in headers.
	for key := range overrides {
		if _, ok := seen[key]; !ok {
			headerKeys = append(headerKeys, key)
		}
	}
	slices.Sort(headerKeys)

	for _, key := range headerKeys {
		_, isSensitive := sensitive[key]
		values := headers[key]
		// If no values exist but we have an override, use that.
		if len(values) == 0 {
			if override, ok := overrides[key]; ok {
				_, err := fmt.Fprintf(w, "%s: %s\r\n", key, override)
				if err != nil {
					return xerrors.Errorf("write response header override: %w", err)
				}
			}
			continue
		}
		for _, value := range values {
			if override, ok := overrides[key]; ok {
				value = override
			}

			if isSensitive {
				value = utils.MaskSecret(value)
			}
			_, err := fmt.Fprintf(w, "%s: %s\r\n", key, value)
			if err != nil {
				return xerrors.Errorf("write response headers: %w", err)
			}
		}
	}
	return nil
}

// interceptDumpPath returns the base file path (without req/resp suffix) for an interception dump.
func interceptDumpPath(baseDir string, provider string, model string, interceptionID uuid.UUID, clk quartz.Clock) string {
	safeModel := strings.ReplaceAll(model, "/", "-")
	return filepath.Join(baseDir, provider, safeModel, fmt.Sprintf("%d-%s", clk.Now().UTC().UnixMilli(), interceptionID))
}

// passthroughDumpPath returns the base file path (without req/resp suffix) for a passthrough dump.
func passthroughDumpPath(baseDir string, provider string, urlPath string, clk quartz.Clock) string {
	safeURLPath := strings.ReplaceAll(strings.TrimPrefix(urlPath, "/"), "/", "-")
	return filepath.Join(baseDir, provider, "passthrough", fmt.Sprintf("%d-%s-%s", clk.Now().UTC().UnixMilli(), safeURLPath, uuid.NewString()[:4]))
}

// NewPassthroughMiddleware returns http.RoundTripper that dumps requests and responses to files.
// If baseDir is empty, returns the original transport unchanged.
// Used for logging in pass through routes.
func NewPassthroughMiddleware(transport http.RoundTripper, baseDir string, provider string, logger slog.Logger, clk quartz.Clock) http.RoundTripper {
	if baseDir == "" {
		return transport
	}
	return &dumpRoundTripper{
		inner:    transport,
		baseDir:  baseDir,
		provider: provider,
		clk:      clk,
		logger:   logger,
	}
}

type dumpRoundTripper struct {
	inner    http.RoundTripper
	baseDir  string
	provider string
	clk      quartz.Clock
	logger   slog.Logger
}

func (rt *dumpRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	dumper := dumper{
		dumpPath: passthroughDumpPath(rt.baseDir, rt.provider, req.URL.Path, rt.clk),
		logger:   rt.logger,
	}

	if err := dumper.dumpRequest(req); err != nil {
		dumper.logger.Named("apidump").Warn(req.Context(), "failed to dump passthrough request", slog.Error(err))
	}

	resp, err := rt.inner.RoundTrip(req)
	if err != nil {
		if dumpErr := dumper.dumpError(err); dumpErr != nil {
			dumper.logger.Named("apidump").Warn(req.Context(), "failed to dump passthrough request error", slog.Error(dumpErr))
		}
		return resp, err
	}

	if err := dumper.dumpResponse(resp); err != nil {
		dumper.logger.Named("apidump").Warn(req.Context(), "failed to dump passthrough response", slog.Error(err))
	}

	return resp, nil
}

// prettyPrintJSON returns indented JSON if body is valid JSON, otherwise returns body as-is.
// Unlike json.MarshalIndent, this preserves the original key order from the input,
// which makes the dumps easier to read and compare with the original requests.
func prettyPrintJSON(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	result := body
	if json.Valid(body) {
		result = pretty.Pretty(body)
	}

	return result
}
