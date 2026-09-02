package codersdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.14.0"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/testutil"
)

const jsonCT = "application/json"

func TestIsConnectionErr(t *testing.T) {
	t.Parallel()

	type tc = struct {
		name           string
		err            error
		expectedResult bool
	}

	cases := []tc{
		{
			// E.g. "no such host"
			name: "DNSError",
			err: &net.DNSError{
				Err:         "no such host",
				Name:        "foofoo",
				Server:      "1.1.1.1:53",
				IsTimeout:   false,
				IsTemporary: false,
				IsNotFound:  true,
			},
			expectedResult: true,
		},
		{
			// E.g. "connection refused"
			name: "OpErr",
			err: &net.OpError{
				Op:     "dial",
				Net:    "tcp",
				Source: nil,
				Addr:   nil,
				Err:    &os.SyscallError{},
			},
			expectedResult: true,
		},
		{
			name:           "OpaqueError",
			err:            xerrors.Errorf("I'm opaque!"),
			expectedResult: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, c.expectedResult, IsConnectionError(c.err))
		})
	}
}

func Test_Client(t *testing.T) {
	t.Parallel()

	const method = http.MethodPost
	const path = "/ok"
	const token = "token"
	const reqBody = `{"msg": "request body"}`
	const resBody = `{"status": "ok"}`

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, method, r.Method)
		assert.Equal(t, path, r.URL.Path)
		assert.Equal(t, token, r.Header.Get(SessionTokenHeader))
		assert.NotEmpty(t, r.Header.Get("Traceparent"))
		for k, v := range r.Header {
			t.Logf("header %q: %q", k, strings.Join(v, ", "))
		}

		w.Header().Set("Content-Type", jsonCT)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, resBody)
	}))

	u, err := url.Parse(s.URL)
	require.NoError(t, err)
	client := New(u)
	client.SetSessionToken(token)

	logBuf := bytes.NewBuffer(nil)
	client.SetLogger(slog.Make(sloghuman.Sink(logBuf)).Leveled(slog.LevelDebug))
	client.SetLogBodies(true)

	// Setup tracing.
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String("codersdk_test"),
	)
	tracerOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
	}
	tracerProvider := sdktrace.NewTracerProvider(tracerOpts...)
	otel.SetTracerProvider(tracerProvider)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {}))
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)
	otel.SetLogger(logr.Discard())
	client.Trace = true

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	ctx, span := tracerProvider.Tracer("codersdk_test").Start(ctx, "codersdk client test 1")
	defer span.End()

	resp, err := client.Request(ctx, method, path, []byte(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, jsonCT, resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, resBody, string(body))

	logStr := logBuf.String()
	require.Contains(t, logStr, "sdk request")
	require.Contains(t, logStr, method)
	require.Contains(t, logStr, path)
	require.Contains(t, logStr, strings.ReplaceAll(reqBody, `"`, `\"`))
	require.Contains(t, logStr, "sdk response")
	require.Contains(t, logStr, "200")
	require.Contains(t, logStr, strings.ReplaceAll(resBody, `"`, `\"`))
}

func Test_Client_LogBodiesFalse(t *testing.T) {
	t.Parallel()

	const method = http.MethodPost
	const path = "/ok"
	const reqBody = `{"msg": "request body"}`
	const resBody = `{"status": "ok"}`

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", jsonCT)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, resBody)
	}))

	u, err := url.Parse(s.URL)
	require.NoError(t, err)
	client := New(u)

	logBuf := bytes.NewBuffer(nil)
	client.SetLogger(slog.Make(sloghuman.Sink(logBuf)).Leveled(slog.LevelDebug))
	client.SetLogBodies(false)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	resp, err := client.Request(ctx, method, path, []byte(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, resBody, string(body))

	logStr := logBuf.String()
	require.Contains(t, logStr, "sdk request")
	require.Contains(t, logStr, "sdk response")
	require.NotContains(t, logStr, "body")
}

func Test_readBodyAsError(t *testing.T) {
	t.Parallel()

	exampleURL := "http://example.com"
	simpleResponse := Response{
		Message: "test",
		Detail:  "hi",
	}

	longResponse := ""
	for i := 0; i < 4000; i++ {
		longResponse += "a"
	}

	unexpectedJSON := marshal(map[string]any{
		"hello": "world",
		"foo":   "bar",
	})

	//nolint:bodyclose
	tests := []struct {
		name   string
		req    *http.Request
		res    *http.Response
		assert func(t *testing.T, err error)
	}{
		{
			name: "JSONWithRequest",
			req:  httptest.NewRequest(http.MethodGet, exampleURL, nil),
			res:  newResponse(http.StatusNotFound, jsonCT, marshal(simpleResponse)),
			assert: func(t *testing.T, err error) {
				sdkErr := assertSDKError(t, err)

				assert.Equal(t, simpleResponse, sdkErr.Response)
				assert.ErrorContains(t, err, sdkErr.Message)
				assert.ErrorContains(t, err, sdkErr.Detail)

				assert.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
				assert.ErrorContains(t, err, strconv.Itoa(sdkErr.StatusCode()))

				assert.Equal(t, http.MethodGet, sdkErr.method)
				assert.ErrorContains(t, err, sdkErr.method)

				assert.Equal(t, exampleURL, sdkErr.url)
				assert.ErrorContains(t, err, sdkErr.url)

				assert.Empty(t, sdkErr.Helper)
			},
		},
		{
			name: "JSONWithoutRequest",
			req:  nil,
			res:  newResponse(http.StatusNotFound, jsonCT, marshal(simpleResponse)),
			assert: func(t *testing.T, err error) {
				sdkErr := assertSDKError(t, err)

				assert.Equal(t, simpleResponse, sdkErr.Response)
				assert.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
				assert.Empty(t, sdkErr.method)
				assert.Empty(t, sdkErr.url)
				assert.Empty(t, sdkErr.Helper)
			},
		},
		{
			name: "UnauthorizedHelper",
			req:  nil,
			res:  newResponse(http.StatusUnauthorized, jsonCT, marshal(simpleResponse)),
			assert: func(t *testing.T, err error) {
				sdkErr := assertSDKError(t, err)

				assert.Contains(t, sdkErr.Helper, "Try logging in")
				assert.ErrorContains(t, err, sdkErr.Helper)
			},
		},
		{
			name: "NonJSON",
			req:  nil,
			res:  newResponse(http.StatusNotFound, "text/plain; charset=utf-8", "hello world"),
			assert: func(t *testing.T, err error) {
				sdkErr := assertSDKError(t, err)

				assert.Contains(t, sdkErr.Message, "unexpected non-JSON response")
				assert.Equal(t, "hello world", sdkErr.Detail)
			},
		},
		{
			name: "NonJSONLong",
			req:  nil,
			res:  newResponse(http.StatusNotFound, "text/plain; charset=utf-8", longResponse),
			assert: func(t *testing.T, err error) {
				sdkErr := assertSDKError(t, err)

				assert.Contains(t, sdkErr.Message, "unexpected non-JSON response")

				expected := longResponse[0:2048] + "..."
				assert.Equal(t, expected, sdkErr.Detail)
			},
		},
		{
			name: "JSONNoBody",
			req:  httptest.NewRequest(http.MethodGet, exampleURL, nil),
			res:  newResponse(http.StatusNotFound, jsonCT, ""),
			assert: func(t *testing.T, err error) {
				sdkErr := assertSDKError(t, err)

				assert.Contains(t, sdkErr.Message, "empty response body")

				assert.Equal(t, http.MethodGet, sdkErr.method)
				assert.ErrorContains(t, err, sdkErr.method)

				assert.Equal(t, exampleURL, sdkErr.url)
				assert.ErrorContains(t, err, sdkErr.url)
			},
		},
		{
			name: "JSONNoMessage",
			req:  nil,
			res:  newResponse(http.StatusNotFound, jsonCT, unexpectedJSON),
			assert: func(t *testing.T, err error) {
				sdkErr := assertSDKError(t, err)

				assert.Contains(t, sdkErr.Message, "unexpected status code")
				assert.Contains(t, sdkErr.Message, "has no message")
				assert.Equal(t, unexpectedJSON, sdkErr.Detail)
			},
		},
		{
			// Even status code 200 should be considered an error if this function
			// is called. There are parts of the code that require this function
			// to always return an error.
			name: "OKResp",
			req:  nil,
			res:  newResponse(http.StatusOK, jsonCT, marshal(map[string]any{})),
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
			},
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			c.res.Request = c.req

			err := ReadBodyAsError(c.res)
			c.assert(t, err)
		})
	}
}

func Test_ReadBodyAsJSON(t *testing.T) {
	t.Parallel()

	type apiResponse struct {
		Name string `json:"name"`
	}

	const htmlBody = `<!DOCTYPE html><html><head><title>Sign in</title></head><body>Sign in to continue</body></html>`

	// assertInvalidBody asserts the fields shared by every error that
	// ReadBodyAsJSON returns for a response with an unusable body.
	assertInvalidBody := func(t *testing.T, err error, contentType string) *Error {
		t.Helper()
		sdkErr := assertSDKError(t, err)
		assert.Equal(t, http.StatusOK, sdkErr.StatusCode())
		assert.Equal(t, http.MethodGet, sdkErr.Method())
		assert.NotEmpty(t, sdkErr.URL())
		assert.Equal(t, contentType, sdkErr.ContentType())
		assert.NotContains(t, err.Error(), "unexpected status code")
		assert.ErrorContains(t, err, "invalid API response (status code 200)")
		return sdkErr
	}

	tests := []struct {
		name    string
		handler http.HandlerFunc
		assert  func(t *testing.T, v apiResponse, err error)
	}{
		{
			name: "ValidJSON",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", jsonCT)
				_, _ = io.WriteString(w, `{"name":"hello"}`)
			},
			assert: func(t *testing.T, v apiResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, "hello", v.Name)
			},
		},
		{
			name: "ValidJSONWithoutContentType",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				// Intermediaries may strip or omit the Content-Type
				// header even when the body is untouched JSON.
				w.Header()["Content-Type"] = nil
				_, _ = io.WriteString(w, `{"name":"hello"}`)
			},
			assert: func(t *testing.T, v apiResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, "hello", v.Name)
			},
		},
		{
			name: "ValidJSONWithWrongContentType",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = io.WriteString(w, `{"name":"hello"}`)
			},
			assert: func(t *testing.T, v apiResponse, err error) {
				require.NoError(t, err)
				assert.Equal(t, "hello", v.Name)
			},
		},
		{
			name: "ValidJSONLargerThanSniffWindow",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", jsonCT)
				_, _ = io.WriteString(w, `{"name":"`+strings.Repeat("a", 4*jsonBodySniffLen)+`"}`)
			},
			assert: func(t *testing.T, v apiResponse, err error) {
				require.NoError(t, err)
				assert.Len(t, v.Name, 4*jsonBodySniffLen)
			},
		},
		{
			name: "HTMLContentType",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = io.WriteString(w, htmlBody)
			},
			assert: func(t *testing.T, _ apiResponse, err error) {
				sdkErr := assertInvalidBody(t, err, "text/html; charset=utf-8")
				assert.Contains(t, sdkErr.Message, "HTML response instead of JSON")
				assert.Contains(t, sdkErr.Helper, "/api/v2")
				assert.NotContains(t, sdkErr.Detail, "<!DOCTYPE html>")
			},
		},
		{
			name: "HTMLMislabeledAsJSON",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", jsonCT)
				_, _ = io.WriteString(w, "\n\t "+htmlBody)
			},
			assert: func(t *testing.T, _ apiResponse, err error) {
				sdkErr := assertInvalidBody(t, err, jsonCT)
				assert.Contains(t, sdkErr.Message, "HTML response instead of JSON")
				assert.Contains(t, sdkErr.Helper, "/api/v2")
			},
		},
		{
			name: "ValidJSONMislabeledAsHTML",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				// An HTML content type is authoritative: Coder API
				// endpoints never serve HTML, so the body is not
				// decoded even when it happens to be valid JSON.
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = io.WriteString(w, `{"name":"hello"}`)
			},
			assert: func(t *testing.T, _ apiResponse, err error) {
				sdkErr := assertInvalidBody(t, err, "text/html; charset=utf-8")
				assert.Contains(t, sdkErr.Message, "HTML response instead of JSON")
			},
		},
		{
			name: "XMLBody",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				// Markup bodies such as XML error pages from load
				// balancers are reported the same way as HTML.
				w.Header().Set("Content-Type", "application/xml")
				_, _ = io.WriteString(w, `<?xml version="1.0"?><error>denied</error>`)
			},
			assert: func(t *testing.T, _ apiResponse, err error) {
				sdkErr := assertInvalidBody(t, err, "application/xml")
				assert.Contains(t, sdkErr.Message, "HTML response instead of JSON")
				assert.NotContains(t, sdkErr.Detail, "<?xml")
			},
		},
		{
			name: "HTMLWithByteOrderMark",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", jsonCT)
				_, _ = io.WriteString(w, "\xef\xbb\xbf"+htmlBody)
			},
			assert: func(t *testing.T, _ apiResponse, err error) {
				sdkErr := assertInvalidBody(t, err, jsonCT)
				assert.Contains(t, sdkErr.Message, "HTML response instead of JSON")
			},
		},
		{
			name: "MalformedJSON",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", jsonCT)
				_, _ = io.WriteString(w, `{"name": "hello"`)
			},
			assert: func(t *testing.T, _ apiResponse, err error) {
				sdkErr := assertInvalidBody(t, err, jsonCT)
				assert.Contains(t, sdkErr.Message, "invalid JSON response")
				assert.NotContains(t, sdkErr.Detail, `{"name": "hello"`)
				assert.Error(t, errors.Unwrap(sdkErr))
			},
		},
		{
			name: "PlainTextBody",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = io.WriteString(w, "upstream connect error")
			},
			assert: func(t *testing.T, _ apiResponse, err error) {
				sdkErr := assertInvalidBody(t, err, "text/plain; charset=utf-8")
				assert.Contains(t, sdkErr.Message, "invalid JSON response")
				assert.NotContains(t, sdkErr.Detail, "upstream connect error")
			},
		},
		{
			name: "EmptyBody",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", jsonCT)
			},
			assert: func(t *testing.T, _ apiResponse, err error) {
				sdkErr := assertInvalidBody(t, err, jsonCT)
				assert.Contains(t, sdkErr.Message, "empty response")
			},
		},
		{
			name: "BodyExcerptOmitted",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = io.WriteString(w, "<html>secret-token")
			},
			assert: func(t *testing.T, _ apiResponse, err error) {
				sdkErr := assertInvalidBody(t, err, "text/html")
				assert.NotContains(t, sdkErr.Detail, "secret-token")
			},
		},
		{
			name: "RedirectToHTML",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/login" {
					http.Redirect(w, r, "/login", http.StatusFound)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = io.WriteString(w, htmlBody)
			},
			assert: func(t *testing.T, _ apiResponse, err error) {
				sdkErr := assertInvalidBody(t, err, "text/html; charset=utf-8")
				assert.Contains(t, sdkErr.Message, "HTML response instead of JSON")
				// The error reports the final URL after redirects and
				// mentions the originally requested URL.
				assert.Contains(t, sdkErr.URL(), "/login")
				assert.Contains(t, sdkErr.Detail, "after following redirects from")
				assert.Contains(t, sdkErr.Detail, "/api/v2/users/me")
			},
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(c.handler)
			defer srv.Close()

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v2/users/me", nil)
			require.NoError(t, err)
			res, err := srv.Client().Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			var v apiResponse
			decodeErr := ReadBodyAsJSON(res, &v)
			c.assert(t, v, decodeErr)
		})
	}

	t.Run("NilResponse", func(t *testing.T) {
		t.Parallel()

		var v apiResponse
		require.Error(t, ReadBodyAsJSON(nil, &v))
	})

	//nolint:bodyclose // The response is constructed, not from a client.
	t.Run("CompleteShortJSONDoesNotReadAhead", func(t *testing.T) {
		t.Parallel()

		errReadAhead := xerrors.New("unexpected read after JSON value")
		body := io.MultiReader(
			strings.NewReader(`{"name":"hello"}`),
			iotest.ErrReader(errReadAhead),
		)
		res := newResponse(http.StatusOK, jsonCT, body)

		var v apiResponse
		require.NoError(t, ReadBodyAsJSON(res, &v))
		require.Equal(t, "hello", v.Name)
	})

	//nolint:bodyclose // The response is constructed, not from a client.
	t.Run("CustomUnmarshalError", func(t *testing.T) {
		t.Parallel()

		var v struct {
			CreatedAt time.Time `json:"created_at"`
		}
		res := newResponse(http.StatusOK, jsonCT, `{"created_at":"invalid"}`)
		requestURL, err := url.Parse("https://coder.example.com/api/v2/users/me")
		require.NoError(t, err)
		res.Request = &http.Request{Method: http.MethodGet, URL: requestURL}
		err = ReadBodyAsJSON(res, &v)
		sdkErr := assertInvalidBody(t, err, jsonCT)
		require.Contains(t, sdkErr.Detail, "cannot parse")
		require.Error(t, errors.Unwrap(sdkErr))
		require.NotContains(t, err.Error(), "read response body")
	})

	//nolint:bodyclose // The response is constructed, not from a client.
	t.Run("BodyReadError", func(t *testing.T) {
		t.Parallel()

		// A transport failure while streaming the body past the sniff
		// window must surface as a read error, not as an invalid API
		// response.
		errTransport := xerrors.New("connection reset by peer")
		body := io.MultiReader(
			strings.NewReader(`{"name":"`+strings.Repeat("a", 2*jsonBodySniffLen)),
			iotest.ErrReader(errTransport),
		)
		res := newResponse(http.StatusOK, jsonCT, body)

		var v apiResponse
		err := ReadBodyAsJSON(res, &v)
		require.ErrorIs(t, err, errTransport)
		require.ErrorContains(t, err, "read response body")
		require.NotContains(t, err.Error(), "invalid API response")
	})

	//nolint:bodyclose // The response is constructed, not from a client.
	t.Run("UseNumber", func(t *testing.T) {
		t.Parallel()

		var v map[string]any
		res := newResponse(http.StatusOK, jsonCT, `{"exp":1750000000}`)
		require.NoError(t, ReadBodyAsJSONUseNumber(res, &v))
		num, ok := v["exp"].(json.Number)
		require.True(t, ok, "expected json.Number, got %T", v["exp"])
		require.Equal(t, "1750000000", num.String())

		// ReadBodyAsJSON decodes the same body's numbers as float64.
		res = newResponse(http.StatusOK, jsonCT, `{"exp":1750000000}`)
		require.NoError(t, ReadBodyAsJSON(res, &v))
		_, ok = v["exp"].(float64)
		require.True(t, ok, "expected float64, got %T", v["exp"])
	})
}

func assertSDKError(t *testing.T, err error) *Error {
	t.Helper()

	var sdkErr *Error
	require.Error(t, err)
	require.True(t, xerrors.As(err, &sdkErr))

	return sdkErr
}

func newResponse(status int, contentType string, body interface{}) *http.Response {
	var r io.ReadCloser
	switch v := body.(type) {
	case string:
		r = io.NopCloser(strings.NewReader(v))
	case []byte:
		r = io.NopCloser(bytes.NewReader(v))
	case io.ReadCloser:
		r = v
	case io.Reader:
		r = io.NopCloser(v)
	default:
		panic(fmt.Sprintf("unknown body type: %T", body))
	}

	return &http.Response{
		Status:     http.StatusText(status),
		StatusCode: status,
		Header: http.Header{
			"Content-Type": []string{contentType},
		},
		Body: r,
	}
}

func marshal(res any) string {
	b, err := json.Marshal(res)
	if err != nil {
		panic(err)
	}

	return string(b)
}

func TestHeaderTransport_Headers(t *testing.T) {
	t.Parallel()

	current := "one"
	inner := &HeaderTransport{
		Transport:  http.DefaultTransport,
		Header:     http.Header{"X-Ignored": {"static is unused when HeaderFunc is set"}},
		HeaderFunc: func() http.Header { return http.Header{"X-Token": {current}} },
	}
	outer := &HeaderTransport{
		Transport: inner,
		Header:    http.Header{"X-Outer": {"yes"}},
	}

	require.Equal(t, http.Header{"X-Token": {"one"}}, inner.Headers())
	require.Equal(t, http.Header{"X-Token": {"one"}, "X-Outer": {"yes"}}, outer.Headers())
	current = "two"
	require.Equal(t, "two", outer.Headers().Get("X-Token"), "outer follows the inner transport's refreshed headers")

	rec := &roundTripRecorder{}
	inner.Transport = rec
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	require.NoError(t, err)
	resp, err := outer.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "two", rec.header.Get("X-Token"))
	require.Equal(t, "yes", rec.header.Get("X-Outer"))
	require.Empty(t, rec.header.Get("X-Ignored"))
	require.Len(t, rec.header.Values("X-Token"), 1, "each layer adds its own headers once")
}

type roundTripRecorder struct {
	header http.Header
}

func (r *roundTripRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.header = req.Header.Clone()
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
}
