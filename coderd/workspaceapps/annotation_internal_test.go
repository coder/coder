package workspaceapps

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk"
)

func TestInjectAnnotationScript(t *testing.T) {
	t.Parallel()

	const scriptTag = `<script src="https://dashboard.example.com/annotator.js"></script>`
	// Larger than the injector's read buffer so markers can straddle reads.
	longBody := "<html><head>" + strings.Repeat("<meta>", 20000) + "</head><body>x</body></html>"

	for _, tc := range []struct {
		name        string
		method      string
		statusCode  int
		contentType string
		encoding    string
		body        string
		want        string
		wantChanged bool
	}{
		{
			name:        "Head",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<html><head><title>t</title></head><body>content</body></html>",
			want:        "<html><head><title>t</title>" + scriptTag + "</head><body>content</body></html>",
			wantChanged: true,
		},
		{
			name:        "BodyWithoutHead",
			statusCode:  http.StatusOK,
			contentType: "text/html",
			body:        "<HTML><BODY>content</BODY></HTML>",
			want:        "<HTML><BODY>content" + scriptTag + "</BODY></HTML>",
			wantChanged: true,
		},
		{
			name:        "HTMLOnly",
			statusCode:  http.StatusOK,
			contentType: "text/html",
			body:        "<html>content</html>",
			want:        "<html>content" + scriptTag + "</html>",
			wantChanged: true,
		},
		{
			name:        "NoMarkers",
			statusCode:  http.StatusOK,
			contentType: "text/html",
			body:        "<p>fragment</p>",
			want:        "<p>fragment</p>" + scriptTag,
			wantChanged: true,
		},
		{
			name:        "Empty",
			statusCode:  http.StatusOK,
			contentType: "text/html",
			body:        "",
			want:        scriptTag,
			wantChanged: true,
		},
		{
			name:        "NonASCIIBeforeMarker",
			statusCode:  http.StatusOK,
			contentType: "text/html",
			body:        "<html><head>İİİ</head><body>ß</body></html>",
			want:        "<html><head>İİİ" + scriptTag + "</head><body>ß</body></html>",
			wantChanged: true,
		},
		{
			name:        "MarkerAcrossReads",
			statusCode:  http.StatusOK,
			contentType: "text/html",
			body:        longBody,
			want:        strings.Replace(longBody, "</head>", scriptTag+"</head>", 1),
			wantChanged: true,
		},
		{
			name:        "NonHTML",
			statusCode:  http.StatusOK,
			contentType: "application/json",
			body:        `{"ok":true}`,
			want:        `{"ok":true}`,
		},
		{
			name:        "Encoded",
			statusCode:  http.StatusOK,
			contentType: "text/html",
			encoding:    "gzip",
			body:        "<html><body>content</body></html>",
			want:        "<html><body>content</body></html>",
		},
		{
			name:        "Non200",
			statusCode:  http.StatusCreated,
			contentType: "text/html",
			body:        "<html><body>content</body></html>",
			want:        "<html><body>content</body></html>",
		},
		{
			name:        "HeadRequest",
			method:      http.MethodHead,
			statusCode:  http.StatusOK,
			contentType: "text/html",
			body:        "",
			want:        "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			method := tc.method
			if method == "" {
				method = http.MethodGet
			}
			resp := &http.Response{
				StatusCode:    tc.statusCode,
				Header:        http.Header{},
				Body:          io.NopCloser(strings.NewReader(tc.body)),
				ContentLength: int64(len(tc.body)),
				Request:       httptest.NewRequest(method, "https://app.example.com/", nil),
			}
			resp.Header.Set("Content-Type", tc.contentType)
			resp.Header.Set("ETag", `"abc"`)
			resp.Header.Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
			if tc.encoding != "" {
				resp.Header.Set("Content-Encoding", tc.encoding)
			}

			require.NoError(t, injectAnnotationScript(resp, scriptTag))
			// Small reads exercise the pending buffer and carry paths.
			got, err := io.ReadAll(iotest.OneByteReader(resp.Body))
			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))
			if tc.wantChanged {
				require.Equal(t, int64(len(tc.want)), resp.ContentLength)
				require.Equal(t, strconv.Itoa(len(tc.want)), resp.Header.Get("Content-Length"))
				require.Empty(t, resp.Header.Get("ETag"), "validators for the upstream bytes must go")
				require.Empty(t, resp.Header.Get("Last-Modified"), "validators for the upstream bytes must go")
			} else {
				require.Equal(t, `"abc"`, resp.Header.Get("ETag"))
				require.NotEmpty(t, resp.Header.Get("Last-Modified"))
			}
		})
	}
}

func TestInjectAnnotationScriptDataWithEOF(t *testing.T) {
	t.Parallel()

	// Some bodies return their final bytes and io.EOF from the same Read;
	// the marker in that last chunk must still be honored.
	const scriptTag = `<script src="https://dashboard.example.com/annotator.js"></script>`
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": {"text/html"}},
		Body:          io.NopCloser(iotest.DataErrReader(strings.NewReader("<html><body>x</body></html>"))),
		ContentLength: -1,
		Request:       httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil),
	}
	require.NoError(t, injectAnnotationScript(resp, scriptTag))
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "<html><body>x"+scriptTag+"</body></html>", string(got))
}

func TestInjectAnnotationScriptStreams(t *testing.T) {
	t.Parallel()

	const scriptTag = `<script src="https://dashboard.example.com/annotator.js"></script>`

	// The upstream writes the head, then stalls. The proxy must hand the
	// rewritten head to the client without waiting for the rest.
	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": {"text/html"}},
		Body:          pr,
		ContentLength: -1,
		Request:       httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil),
	}
	require.NoError(t, injectAnnotationScript(resp, scriptTag))
	require.Equal(t, int64(-1), resp.ContentLength)
	require.Empty(t, resp.Header.Get("Content-Length"))

	go func() {
		_, _ = pw.Write([]byte("<html><head></head><body>"))
	}()
	first := make([]byte, 1024)
	n, err := resp.Body.Read(first)
	require.NoError(t, err)
	require.Equal(t, "<html><head>"+scriptTag+"</head><body>", string(first[:n]))

	go func() {
		_, _ = pw.Write([]byte("tail</body></html>"))
		_ = pw.Close()
	}()
	rest, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "tail</body></html>", string(rest))
}

// errAfterReader returns its whole payload together with an error, the
// way a connection that drops mid-body can.
type errAfterReader struct {
	data string
	err  error
	done bool
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	return copy(p, r.data), r.err
}

func TestInjectAnnotationScriptUpstreamError(t *testing.T) {
	t.Parallel()

	const scriptTag = `<script src="https://dashboard.example.com/annotator.js"></script>`
	errUpstream := xerrors.New("upstream reset")
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": {"text/html"}},
		Body:          io.NopCloser(&errAfterReader{data: "<html><body>partial", err: errUpstream}),
		ContentLength: -1,
		Request:       httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil),
	}
	require.NoError(t, injectAnnotationScript(resp, scriptTag))

	// The bytes that arrived are delivered untouched, then the error; the
	// tag is never appended to a body that did not finish.
	got, err := io.ReadAll(resp.Body)
	require.ErrorIs(t, err, errUpstream)
	require.Equal(t, "<html><body>partial", string(got))
}

func TestAnnotationScriptTag(t *testing.T) {
	t.Parallel()

	version := url.QueryEscape(buildinfo.Version())
	for _, tc := range []struct {
		name string
		url  string
		want string
	}{
		{
			name: "TrailingSlash",
			url:  "https://dashboard.example.com/coder/",
			want: `<script src="https://dashboard.example.com/coder/annotator.js?v=` + version + `" data-coder-origin="https://dashboard.example.com" defer></script>`,
		},
		{
			name: "Escaping",
			url:  "https://dashboard.example.com/<script>?ignored=value",
			want: `<script src="https://dashboard.example.com/%3Cscript%3E/annotator.js?v=` + version + `" data-coder-origin="https://dashboard.example.com" defer></script>`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dashboardURL, err := url.Parse(tc.url)
			require.NoError(t, err)
			require.Equal(t, tc.want, annotationScriptTag(dashboardURL))
		})
	}
}

func TestAnnotationRequested(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		experiments  codersdk.Experiments
		accessMethod AccessMethod
		rawQuery     string
		want         bool
		wantQuery    string
	}{
		{
			name:         "Requested",
			experiments:  codersdk.Experiments{codersdk.ExperimentChatUIAnnotations},
			accessMethod: AccessMethodSubdomain,
			rawQuery:     AnnotationQueryParam + "=1&keep=value",
			want:         true,
			wantQuery:    "keep=value",
		},
		{
			name:         "Disabled",
			experiments:  codersdk.Experiments{codersdk.ExperimentChatUIAnnotations},
			accessMethod: AccessMethodSubdomain,
			rawQuery:     AnnotationQueryParam + "=0",
			wantQuery:    "",
		},
		{
			name:         "Absent",
			experiments:  codersdk.Experiments{codersdk.ExperimentChatUIAnnotations},
			accessMethod: AccessMethodSubdomain,
			rawQuery:     "keep=value",
			wantQuery:    "keep=value",
		},
		{
			// The marker is proxy-owned and never reaches the app, even
			// when it cannot be honored.
			name:         "ExperimentDisabled",
			accessMethod: AccessMethodSubdomain,
			rawQuery:     AnnotationQueryParam + "=1&keep=value",
			wantQuery:    "keep=value",
		},
		{
			name:         "PathAccess",
			experiments:  codersdk.Experiments{codersdk.ExperimentChatUIAnnotations},
			accessMethod: AccessMethodPath,
			rawQuery:     AnnotationQueryParam + "=1",
			wantQuery:    "",
		},
		{
			// Everything but the marker is forwarded exactly as sent: no
			// reordering, no re-escaping, and no dropping of pairs Go's
			// query parser rejects.
			name:         "PreservesQueryVerbatim",
			experiments:  codersdk.Experiments{codersdk.ExperimentChatUIAnnotations},
			accessMethod: AccessMethodSubdomain,
			rawQuery:     "z=1&path=%2Fa+b&" + AnnotationQueryParam + "=1&flag&legacy=a;b",
			want:         true,
			wantQuery:    "z=1&path=%2Fa+b&flag&legacy=a;b",
		},
		{
			name:         "EncodedKeyAndRepeats",
			experiments:  codersdk.Experiments{codersdk.ExperimentChatUIAnnotations},
			accessMethod: AccessMethodSubdomain,
			rawQuery:     "coder%5Fannotate=1&" + AnnotationQueryParam + "=0",
			want:         true,
			wantQuery:    "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := &Server{ServerOptions: ServerOptions{Experiments: tc.experiments}}
			req := httptest.NewRequest(http.MethodGet, "https://app.example.com/preview?"+tc.rawQuery, nil)

			require.Equal(t, tc.want, srv.annotationRequested(req, tc.accessMethod))
			require.Equal(t, tc.wantQuery, req.URL.RawQuery)
		})
	}
}
