package workspaceapps

import (
	"bytes"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/coder/coder/v2/codersdk"
)

const annotationResponseBodyLimit = 8 << 20

func (s *Server) annotationsEnabled() bool {
	return s.Experiments.Enabled(codersdk.ExperimentChatUIAnnotations)
}

// annotationRequested reports whether the dashboard asked for the
// annotation overlay on this request and removes the marker parameter so
// the upstream app never sees it. Only the request carrying the parameter
// is rewritten; client-side navigation inside the app keeps the injected
// script, while a full navigation drops it until the dashboard asks again.
func (s *Server) annotationRequested(r *http.Request, accessMethod AccessMethod) bool {
	if !s.annotationsEnabled() || accessMethod != AccessMethodSubdomain {
		return false
	}
	query := r.URL.Query()
	if !query.Has(AnnotationQueryParam) {
		return false
	}
	requested := query.Get(AnnotationQueryParam) == "1"
	query.Del(AnnotationQueryParam)
	r.URL.RawQuery = query.Encode()
	return requested
}

func injectAnnotationScript(resp *http.Response, scriptTag string) error {
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Encoding") != "" {
		return nil
	}

	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "text/html") {
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, annotationResponseBodyLimit+1))
	if err != nil {
		resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), resp.Body))
		return err
	}
	if len(body) > annotationResponseBodyLimit {
		resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), resp.Body))
		return nil
	}

	lowerBody := bytes.ToLower(body)
	insertionIndex := bytes.LastIndex(lowerBody, []byte("</body>"))
	if insertionIndex < 0 {
		insertionIndex = bytes.LastIndex(lowerBody, []byte("</html>"))
	}
	if insertionIndex < 0 {
		insertionIndex = len(body)
	}

	body = bytes.Join([][]byte{body[:insertionIndex], []byte(scriptTag), body[insertionIndex:]}, nil)
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	return nil
}

func annotationScriptTag(dashboardURL *url.URL) string {
	dashboard := *dashboardURL
	dashboard.Path = strings.TrimRight(dashboard.Path, "/") + "/annotator.js"
	dashboard.RawPath = ""
	dashboard.RawQuery = ""
	dashboard.Fragment = ""

	src := dashboard.String()
	origin := dashboard.Scheme + "://" + dashboard.Host
	return `<script src="` + html.EscapeString(src) + `" data-coder-origin="` + html.EscapeString(origin) + `" defer></script>`
}
