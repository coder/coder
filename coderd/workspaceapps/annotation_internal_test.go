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
	// Larger than the injector's read buffer.
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
			contentType: "text/html; charset=utf-8",
			body:        "<HTML><BODY>content</BODY></HTML>",
			want:        "<HTML>" + scriptTag + "<BODY>content</BODY></HTML>",
			wantChanged: true,
		},
		{
			name:        "HTMLOnly",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<html>content</html>",
			want:        "<html>content</html>",
			wantChanged: true,
		},
		{
			name:        "NoMarkers",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<p>fragment</p>",
			want:        "<p>fragment</p>",
			wantChanged: true,
		},
		{
			name:        "Empty",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "",
			want:        "",
			wantChanged: true,
		},
		{
			name:        "NonASCIIBeforeMarker",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<html><head><title>İİİ</title></head><body>ß</body></html>",
			want:        "<html><head><title>İİİ</title>" + scriptTag + "</head><body>ß</body></html>",
			wantChanged: true,
		},
		{
			name:        "LargeBody",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        longBody,
			want:        strings.Replace(longBody, "</head>", scriptTag+"</head>", 1),
			wantChanged: true,
		},
		{
			name:        "JSONScript",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<html><head><script type="application/json">{"name":"</head><img src=x onerror=alert(1)>"}</script></head><body></body></html>`,
			want:        `<html><head><script type="application/json">{"name":"</head><img src=x onerror=alert(1)>"}</script>` + scriptTag + `</head><body></body></html>`,
			wantChanged: true,
		},
		{
			name:        "InlineScript",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<html><head><script>const value = "</head><img src=x onerror=alert(1)>";</script></head></html>`,
			want:        `<html><head><script>const value = "</head><img src=x onerror=alert(1)>";</script>` + scriptTag + `</head></html>`,
			wantChanged: true,
		},
		{
			name:        "HeadTextContexts",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<html><head><!-- </head> --><title></body></title><style>p::before { content: "</html>"; }</style><meta name="test" content="</head>"></head></html>`,
			want:        `<html><head><!-- </head> --><title></body></title><style>p::before { content: "</html>"; }</style><meta name="test" content="</head>">` + scriptTag + `</head></html>`,
			wantChanged: true,
		},
		{
			name:        "UnterminatedScript",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<html><head><script type="application/json">{"name":"</head><img src=x onerror=alert(1)>"}`,
			want:        `<html><head><script type="application/json">{"name":"</head><img src=x onerror=alert(1)>"}`,
			wantChanged: true,
		},
		{
			name:        "ForeignContent",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<html><head><svg><title><script type="application/json">{"name":"</title></head><img src=x onerror=alert(1)>"}</script></title></svg></head></html>`,
			want:        `<html><head><svg><title><script type="application/json">{"name":"</title></head><img src=x onerror=alert(1)>"}</script></title></svg></head></html>`,
			wantChanged: true,
		},
		{
			name:        "NoCharset",
			statusCode:  http.StatusOK,
			contentType: "text/html",
			body:        "<html><head></head><body></body></html>",
			want:        "<html><head></head><body></body></html>",
		},
		{
			name:        "OtherCharset",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=iso-2022-jp",
			body:        "<html><head></head><body></body></html>",
			want:        "<html><head></head><body></body></html>",
		},
		{
			name:        "ImplicitHead",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=UTF-8",
			body:        "<!doctype html><meta charset=utf-8><title>Title</title><body>content</body>",
			want:        "<!doctype html><meta charset=utf-8><title>Title</title>" + scriptTag + "<body>content</body>",
			wantChanged: true,
		},
		{
			name:        "UTF8BOM",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf8",
			body:        "\xef\xbb\xbf<!doctype html>\r\n<HTML><HEAD><META content='A&amp;B'></HEAD></HTML>",
			want:        "\xef\xbb\xbf<!doctype html>\r\n<HTML><HEAD><META content='A&amp;B'>" + scriptTag + "</HEAD></HTML>",
			wantChanged: true,
		},
		{
			name:        "CSPMeta",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        `<head><meta http-equiv="Content-Security-Policy" content="script-src 'none'"></head><body></body>`,
			want:        `<head><meta http-equiv="Content-Security-Policy" content="script-src 'none'">` + scriptTag + `</head><body></body>`,
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
			contentType: "text/html; charset=utf-8",
			encoding:    "gzip",
			body:        "<html><body>content</body></html>",
			want:        "<html><body>content</body></html>",
		},
		{
			name:        "Non200",
			statusCode:  http.StatusCreated,
			contentType: "text/html; charset=utf-8",
			body:        "<html><body>content</body></html>",
			want:        "<html><body>content</body></html>",
		},
		{
			name:        "HeadRequest",
			method:      http.MethodHead,
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
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
				Body:          io.NopCloser(iotest.OneByteReader(strings.NewReader(tc.body))),
				ContentLength: int64(len(tc.body)),
				Request:       httptest.NewRequest(method, "https://app.example.com/", nil),
			}
			resp.Header.Set("Content-Type", tc.contentType)
			resp.Header.Set("Content-Length", strconv.Itoa(len(tc.body)))
			resp.Header.Set("Content-Security-Policy", "default-src 'self'")
			resp.Header.Set("ETag", `"abc"`)
			resp.Header.Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
			if tc.encoding != "" {
				resp.Header.Set("Content-Encoding", tc.encoding)
			}

			require.NoError(t, injectAnnotationScript(resp, scriptTag))
			// Small reads exercise buffering on both sides of the transformer.
			got, err := io.ReadAll(iotest.OneByteReader(resp.Body))
			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))
			require.Equal(t, "default-src 'self'", resp.Header.Get("Content-Security-Policy"))
			if tc.wantChanged {
				require.Equal(t, int64(-1), resp.ContentLength)
				require.Empty(t, resp.Header.Get("Content-Length"))
				require.Empty(t, resp.Header.Get("ETag"), "validators for the upstream bytes must go")
				require.Empty(t, resp.Header.Get("Last-Modified"), "validators for the upstream bytes must go")
			} else {
				require.Equal(t, int64(len(tc.body)), resp.ContentLength)
				require.Equal(t, strconv.Itoa(len(tc.body)), resp.Header.Get("Content-Length"))
				require.Equal(t, `"abc"`, resp.Header.Get("ETag"))
				require.NotEmpty(t, resp.Header.Get("Last-Modified"))
			}
		})
	}
}

func TestInjectAnnotationScriptPassthrough(t *testing.T) {
	t.Parallel()

	const scriptTag = `<script src="https://dashboard.example.com/annotator.js"></script>`
	for _, tc := range []struct {
		name string
		body string
	}{
		{"UnterminatedComment", `<head><!-- </head><body>`},
		{"UnterminatedAttribute", `<head><meta content="</head><body>`},
		{"Template", `<head><template><head></head></template></head>`},
		{"Noscript", `<head><noscript></head></noscript></head>`},
		{"Select", `<head><select><style><script>"</style></head>"</script></select>`},
		{"MathML", `<head><math><annotation-xml><head></head></annotation-xml></math></head>`},
		{"SelfClosingScript", `<head><script/>"</head>"</script></head>`},
		{"ImplicitBodyText", `<head>body text</head><body>`},
		{"UnicodeWhitespace", "<head>\u00a0</head><body>"},
		{"EarlyHTMLClose", `<html></html><head></head>`},
		{"RepeatedHead", `<head><head></head>`},
		{"NUL", "<head>\x00</head>"},
		{"InvalidUTF8", "<head><title>\xff</title></head>"},
		{"UTF16BOM", "\xff\xfe<\x00h\x00t\x00m\x00l\x00>\x00"},
		{"OversizedScript", `<head><script>` + strings.Repeat("x", annotationMaxTokenBytes) + `</script></head>`},
		{"OversizedAttribute", `<head><meta content="` + strings.Repeat("x", annotationMaxTokenBytes) + `"></head>`},
		{"OversizedComment", `<head><!--` + strings.Repeat("x", annotationMaxTokenBytes) + `--></head>`},
		{"PrefixBudget", `<head>` + strings.Repeat("<meta>", annotationMaxPrefixBytes/len("<meta>")) + `</head>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := &http.Response{
				StatusCode:    http.StatusOK,
				Header:        http.Header{"Content-Type": {"text/html; charset=utf-8"}},
				Body:          io.NopCloser(strings.NewReader(tc.body)),
				ContentLength: int64(len(tc.body)),
				Request:       httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil),
			}
			require.NoError(t, injectAnnotationScript(resp, scriptTag))
			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.body, string(got), "unsupported contexts must pass through byte for byte")
		})
	}
}

func TestInjectAnnotationScriptMarkerAcrossReads(t *testing.T) {
	t.Parallel()

	const scriptTag = `<script src="https://dashboard.example.com/annotator.js"></script>`
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "Head",
			body: "<html><head><title>Title</title></head><body>content</body></html>",
			want: "<html><head><title>Title</title>" + scriptTag + "</head><body>content</body></html>",
		},
		{
			name: "Body",
			body: "<html><body>content</body></html>",
			want: "<html>" + scriptTag + "<body>content</body></html>",
		},
		{
			name: "Script",
			body: `<head><script type="application/json">{"name":"</head><img src=x onerror=alert(1)>"}</script></head>`,
			want: `<head><script type="application/json">{"name":"</head><img src=x onerror=alert(1)>"}</script>` + scriptTag + `</head>`,
		},
	} {
		for split := 1; split < len(tc.body); split++ {
			t.Run(tc.name+"/"+strconv.Itoa(split), func(t *testing.T) {
				t.Parallel()

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"text/html; charset=utf-8"}},
					Body: io.NopCloser(io.MultiReader(
						strings.NewReader(tc.body[:split]),
						strings.NewReader(tc.body[split:]),
					)),
					ContentLength: -1,
					Request:       httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil),
				}
				require.NoError(t, injectAnnotationScript(resp, scriptTag))

				got, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, tc.want, string(got))
			})
		}
	}
}

func TestInjectAnnotationScriptDataWithEOF(t *testing.T) {
	t.Parallel()

	// Some bodies return their final bytes and io.EOF from the same Read;
	// the marker in that last chunk must still be honored.
	const scriptTag = `<script src="https://dashboard.example.com/annotator.js"></script>`
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": {"text/html; charset=utf-8"}},
		Body:          io.NopCloser(iotest.DataErrReader(strings.NewReader("<html><body>x</body></html>"))),
		ContentLength: -1,
		Request:       httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil),
	}
	require.NoError(t, injectAnnotationScript(resp, scriptTag))
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "<html>"+scriptTag+"<body>x</body></html>", string(got))
}

func TestInjectAnnotationScriptStreams(t *testing.T) {
	t.Parallel()

	const scriptTag = `<script src="https://dashboard.example.com/annotator.js"></script>`
	longScript := "<html><head><script>" + strings.Repeat("x", 2*annotationMaxTokenBytes)
	for _, tc := range []struct {
		name      string
		first     string
		wantFirst string
		tail      string
	}{
		{
			name:      "Head",
			first:     "<html><head></head><body>",
			wantFirst: "<html><head>" + scriptTag + "</head><body>",
			tail:      "tail</body></html>",
		},
		{
			name:      "OversizedToken",
			first:     longScript,
			wantFirst: longScript,
			tail:      "</script></head><body>tail</body></html>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// The upstream stalls after its first flush. Both injection and
			// oversized-token passthrough must progress without waiting for EOF.
			pr, pw := io.Pipe()
			t.Cleanup(func() {
				_ = pr.Close()
				_ = pw.Close()
			})
			resp := &http.Response{
				StatusCode:    http.StatusOK,
				Header:        http.Header{"Content-Type": {"text/html; charset=utf-8"}},
				Body:          pr,
				ContentLength: -1,
				Request:       httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil),
			}
			require.NoError(t, injectAnnotationScript(resp, scriptTag))
			require.Equal(t, int64(-1), resp.ContentLength)
			require.Empty(t, resp.Header.Get("Content-Length"))

			go func() {
				_, _ = pw.Write([]byte(tc.first))
			}()
			first := make([]byte, len(tc.wantFirst))
			_, err := io.ReadFull(resp.Body, first)
			require.NoError(t, err)
			require.Equal(t, tc.wantFirst, string(first))

			go func() {
				_, _ = pw.Write([]byte(tc.tail))
				_ = pw.Close()
			}()
			rest, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.tail, string(rest))
		})
	}
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
		return 0, io.EOF
	}
	r.done = true
	return copy(p, r.data), r.err
}

func TestInjectAnnotationScriptUpstreamError(t *testing.T) {
	t.Parallel()

	const scriptTag = `<script src="https://dashboard.example.com/annotator.js"></script>`
	errUpstream := xerrors.New("upstream reset")
	for _, body := range []string{
		"<html><body>partial",
		"<html><head></head><body>partial",
	} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()

			resp := &http.Response{
				StatusCode:    http.StatusOK,
				Header:        http.Header{"Content-Type": {"text/html; charset=utf-8"}},
				Body:          io.NopCloser(&errAfterReader{data: body, err: errUpstream}),
				ContentLength: -1,
				Request:       httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil),
			}
			require.NoError(t, injectAnnotationScript(resp, scriptTag))

			// Deliver all bytes, then the original error, even if a later source
			// read returns EOF instead of repeating the error.
			got, err := io.ReadAll(resp.Body)
			require.ErrorIs(t, err, errUpstream)
			require.Equal(t, body, string(got))
		})
	}
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
