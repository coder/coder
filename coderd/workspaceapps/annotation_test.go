package workspaceapps //nolint:testpackage // Tests unexported annotation helpers.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestInjectAnnotationScript(t *testing.T) {
	t.Parallel()

	const scriptTag = `<script src="https://dashboard.example.com/annotator.js"></script>`
	overLimitBody := strings.Repeat("x", annotationResponseBodyLimit+1)

	for _, tc := range []struct {
		name        string
		statusCode  int
		contentType string
		encoding    string
		body        string
		want        string
		wantChanged bool
	}{
		{
			name:        "Body",
			statusCode:  http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<html><body>content</body></html>",
			want:        "<html><body>content" + scriptTag + "</body></html>",
			wantChanged: true,
		},
		{
			name:        "UppercaseBody",
			statusCode:  http.StatusOK,
			contentType: "text/html",
			body:        "<HTML><BODY>content</BODY></HTML>",
			want:        "<HTML><BODY>content" + scriptTag + "</BODY></HTML>",
			wantChanged: true,
		},
		{
			name:        "NoBody",
			statusCode:  http.StatusOK,
			contentType: "text/html",
			body:        "<html>content</html>",
			want:        "<html>content" + scriptTag + "</html>",
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
			name:        "OverLimit",
			statusCode:  http.StatusOK,
			contentType: "text/html",
			body:        overLimitBody,
			want:        overLimitBody,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := &http.Response{
				StatusCode:    tc.statusCode,
				Header:        http.Header{"Content-Type": {tc.contentType}},
				Body:          io.NopCloser(strings.NewReader(tc.body)),
				ContentLength: int64(len(tc.body)),
			}
			if tc.encoding != "" {
				resp.Header.Set("Content-Encoding", tc.encoding)
			}

			require.NoError(t, injectAnnotationScript(resp, scriptTag))
			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))
			if tc.wantChanged {
				require.Equal(t, int64(len(tc.want)), resp.ContentLength)
				require.Equal(t, strconv.Itoa(len(tc.want)), resp.Header.Get("Content-Length"))
			}
		})
	}
}

func TestAnnotationScriptTag(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		url  string
		want string
	}{
		{
			name: "TrailingSlash",
			url:  "https://dashboard.example.com/coder/",
			want: `<script src="https://dashboard.example.com/coder/annotator.js" data-coder-origin="https://dashboard.example.com" defer></script>`,
		},
		{
			name: "Escaping",
			url:  "https://dashboard.example.com/<script>?ignored=value",
			want: `<script src="https://dashboard.example.com/%3Cscript%3E/annotator.js" data-coder-origin="https://dashboard.example.com" defer></script>`,
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
			name:         "ExperimentDisabled",
			accessMethod: AccessMethodSubdomain,
			rawQuery:     AnnotationQueryParam + "=1",
			wantQuery:    AnnotationQueryParam + "=1",
		},
		{
			name:         "PathAccess",
			experiments:  codersdk.Experiments{codersdk.ExperimentChatUIAnnotations},
			accessMethod: AccessMethodPath,
			rawQuery:     AnnotationQueryParam + "=1",
			wantQuery:    AnnotationQueryParam + "=1",
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
