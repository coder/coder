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

func TestHandleAnnotationParam(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		experiments  codersdk.Experiments
		accessMethod AccessMethod
		query        url.Values
		handled      bool
		cookieValue  string
		cookieMaxAge int
		location     string
	}{
		{
			name:         "SetsCookieAndRedirects",
			experiments:  codersdk.Experiments{codersdk.ExperimentChatUIAnnotations},
			accessMethod: AccessMethodSubdomain,
			query:        url.Values{AnnotationQueryParam: {"1"}, "keep": {"value"}},
			handled:      true,
			cookieValue:  "1",
			cookieMaxAge: 0,
			location:     "/preview?keep=value",
		},
		{
			name:         "ClearsCookie",
			experiments:  codersdk.Experiments{codersdk.ExperimentChatUIAnnotations},
			accessMethod: AccessMethodSubdomain,
			query:        url.Values{AnnotationQueryParam: {"0"}},
			handled:      true,
			cookieMaxAge: -1,
			location:     "/preview",
		},
		{
			name:         "ExperimentDisabled",
			accessMethod: AccessMethodSubdomain,
			query:        url.Values{AnnotationQueryParam: {"1"}},
		},
		{
			name:         "PathAccess",
			experiments:  codersdk.Experiments{codersdk.ExperimentChatUIAnnotations},
			accessMethod: AccessMethodPath,
			query:        url.Values{AnnotationQueryParam: {"1"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := &Server{ServerOptions: ServerOptions{Experiments: tc.experiments}}
			req := httptest.NewRequest(http.MethodGet, "https://app.example.com/preview?"+tc.query.Encode(), nil)
			rec := httptest.NewRecorder()

			require.Equal(t, tc.handled, srv.handleAnnotationParam(rec, req, tc.accessMethod))
			if !tc.handled {
				require.Equal(t, http.StatusOK, rec.Code)
				return
			}

			res := rec.Result()
			defer res.Body.Close()
			require.Equal(t, http.StatusSeeOther, res.StatusCode)
			require.Equal(t, tc.location, res.Header.Get("Location"))

			cookies := res.Cookies()
			require.Len(t, cookies, 1)
			require.Equal(t, codersdk.AppAnnotationCookie, cookies[0].Name)
			require.Equal(t, tc.cookieValue, cookies[0].Value)
			require.Equal(t, tc.cookieMaxAge, cookies[0].MaxAge)
			require.Equal(t, "/", cookies[0].Path)
			require.Empty(t, cookies[0].Domain)
			require.True(t, cookies[0].HttpOnly)
		})
	}
}

func TestAnnotationRequested(t *testing.T) {
	t.Parallel()

	srv := &Server{ServerOptions: ServerOptions{
		Experiments: codersdk.Experiments{codersdk.ExperimentChatUIAnnotations},
	}}
	req := httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil)
	req.AddCookie(&http.Cookie{Name: codersdk.AppAnnotationCookie, Value: "1"})
	require.True(t, srv.annotationRequested(req, AccessMethodSubdomain))
	require.False(t, srv.annotationRequested(req, AccessMethodPath))
}
