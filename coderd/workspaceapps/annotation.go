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

func (s *Server) handleAnnotationParam(rw http.ResponseWriter, r *http.Request, accessMethod AccessMethod) (handled bool) {
	if !s.annotationsEnabled() || accessMethod != AccessMethodSubdomain || !r.URL.Query().Has(AnnotationQueryParam) {
		return false
	}

	value, maxAge := "", -1
	if r.URL.Query().Get(AnnotationQueryParam) == "1" {
		value, maxAge = "1", 0
	}
	http.SetCookie(rw, s.CookiesConfig.Apply(&http.Cookie{
		Name:     codersdk.AppAnnotationCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
	}))

	redirectURL := originLocalURL(r.URL.Path)
	query := r.URL.Query()
	query.Del(AnnotationQueryParam)
	redirectURL.RawQuery = query.Encode()
	http.Redirect(rw, r, redirectURL.String(), http.StatusSeeOther)
	return true
}

func (s *Server) annotationRequested(r *http.Request, accessMethod AccessMethod) bool {
	if !s.annotationsEnabled() || accessMethod != AccessMethodSubdomain {
		return false
	}
	cookie, err := r.Cookie(codersdk.AppAnnotationCookie)
	return err == nil && cookie.Value == "1"
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
