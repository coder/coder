package workspaceapps

import (
	"bytes"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk"
)

func (s *Server) annotationsEnabled() bool {
	return s.Experiments.Enabled(codersdk.ExperimentChatUIAnnotations)
}

// annotationRequested reports whether the dashboard asked for the
// annotation overlay on this request. The marker parameter is reserved by
// the proxy and is always removed so the upstream app never sees it; the
// overlay is only injected for subdomain apps while the experiment is on.
// Only the request carrying the parameter is rewritten; client-side
// navigation inside the app keeps the injected script, while a full
// navigation drops it until the dashboard asks again.
func (s *Server) annotationRequested(r *http.Request, accessMethod AccessMethod) bool {
	value, found, rest := stripAnnotationParam(r.URL.RawQuery)
	if !found {
		return false
	}
	r.URL.RawQuery = rest
	return value == "1" && s.annotationsEnabled() && accessMethod == AccessMethodSubdomain
}

// stripAnnotationParam removes the marker from a raw query string and
// returns its value along with the remaining query. The rest is left
// byte for byte as the client sent it rather than re-encoded through
// url.Values, which would reorder parameters, normalise escapes and drop
// pairs Go does not parse; none of that is the proxy's to change.
func stripAnnotationParam(rawQuery string) (value string, found bool, rest string) {
	if rawQuery == "" {
		return "", false, rawQuery
	}
	segments := strings.Split(rawQuery, "&")
	kept := make([]string, 0, len(segments))
	for _, segment := range segments {
		rawKey, rawValue, _ := strings.Cut(segment, "=")
		if key, err := url.QueryUnescape(rawKey); err != nil || key != AnnotationQueryParam {
			kept = append(kept, segment)
			continue
		}
		if !found {
			found = true
			value, _ = url.QueryUnescape(rawValue)
		}
	}
	return value, found, strings.Join(kept, "&")
}

// injectAnnotationScript rewrites a successful, uncompressed UTF-8 HTML GET
// response so that it loads the overlay script. Only a bounded document
// prologue is inspected; unsupported HTML passes through without injection.
func injectAnnotationScript(resp *http.Response, scriptTag string) error {
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Encoding") != "" {
		return nil
	}
	if resp.Request != nil && resp.Request.Method != http.MethodGet {
		return nil
	}
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "text/html") {
		return nil
	}
	// An explicit UTF-8 charset keeps the tokenizer and browser in agreement.
	// Without it the browser can sniff a different encoding from the body.
	if !strings.EqualFold(params["charset"], "utf-8") && !strings.EqualFold(params["charset"], "utf8") {
		return nil
	}

	src := &annotationSource{ReadCloser: resp.Body}
	tokenizer := xhtml.NewTokenizer(src)
	tokenizer.SetMaxBuf(annotationMaxTokenBytes)
	resp.Body = &annotationInjector{src: src, tag: []byte(scriptTag), tokenizer: tokenizer}
	// Injection can be declined after headers have been sent, so its length
	// cannot be included in Content-Length in advance.
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
	// These describe the upstream bytes, which no longer match. Last-Modified
	// goes too: the rewritten page also depends on the Coder version in the
	// script URL, which a 304 against the upstream's date would preserve.
	for _, header := range []string{"ETag", "Last-Modified", "Content-MD5", "Digest", "Content-Digest", "Repr-Digest", "Accept-Ranges"} {
		resp.Header.Del(header)
	}
	return nil
}

const (
	annotationMaxTokenBytes  = 64 << 10
	annotationMaxPrefixBytes = 1 << 20
)

// annotationSource retains errors returned alongside bytes. Switching from
// tokenization to passthrough must not lose an error held by the tokenizer.
type annotationSource struct {
	io.ReadCloser
	err error
}

func (s *annotationSource) Read(p []byte) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	n, err := s.ReadCloser.Read(p)
	s.err = err
	return n, err
}

// annotationInjector inserts before a head end tag or an opening body tag.
// It never searches beyond the document prologue: tokenization alone cannot
// model the browser's tree-dependent parsing of arbitrary body/foreign HTML.
type annotationInjector struct {
	src       *annotationSource
	tag       []byte
	tokenizer *xhtml.Tokenizer
	pending   []byte
	rest      io.Reader

	scanned     int
	htmlSeen    bool
	headStarted bool
	rawTag      string
}

func (a *annotationInjector) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(a.pending) == 0 {
		if a.rest != nil {
			return a.rest.Read(p)
		}
		a.fill()
	}
	n := copy(p, a.pending)
	a.pending = a.pending[n:]
	return n, nil
}

func (a *annotationInjector) fill() {
	tt := a.tokenizer.Next()
	// TagName and other tokenizer accessors can mutate Raw's backing buffer.
	// Preserve the exact upstream bytes before inspecting the token.
	a.pending = bytes.Clone(a.tokenizer.Raw())
	a.scanned += len(a.pending)
	if tt == xhtml.ErrorToken {
		if err := a.tokenizer.Err(); !xerrors.Is(err, xhtml.ErrBufferExceeded) {
			a.src.err = err
		}
		a.passthrough()
		return
	}
	if a.scanned > annotationMaxPrefixBytes || !utf8.Valid(a.pending) || bytes.IndexByte(a.pending, 0) >= 0 || (a.src.err != nil && !xerrors.Is(a.src.err, io.EOF)) {
		a.passthrough()
		return
	}

	inject, valid := a.inspect(tt)
	if inject {
		a.pending = bytes.Join([][]byte{a.tag, a.pending}, nil)
	}
	if inject || !valid {
		a.passthrough()
	}
}

// inspect accepts only the prologue and ordinary head elements. Everything
// else permanently disables injection rather than guessing a safe context.
func (a *annotationInjector) inspect(tt xhtml.TokenType) (inject, valid bool) {
	if a.rawTag != "" {
		if tt == xhtml.TextToken {
			return false, true
		}
		if tt == xhtml.EndTagToken {
			name, _ := a.tokenizer.TagName()
			if string(name) == a.rawTag {
				a.rawTag = ""
				return false, true
			}
		}
		return false, false
	}

	switch tt {
	case xhtml.TextToken:
		text := a.pending
		if a.scanned == len(text) {
			text = bytes.TrimPrefix(text, []byte("\xef\xbb\xbf"))
		}
		return false, len(bytes.Trim(text, " \t\r\n\f")) == 0
	case xhtml.CommentToken:
		return false, true
	case xhtml.DoctypeToken:
		return false, !a.htmlSeen && !a.headStarted
	case xhtml.EndTagToken:
		name, _ := a.tokenizer.TagName()
		return string(name) == "head" && a.headStarted, false
	case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
		name, _ := a.tokenizer.TagName()
		switch string(name) {
		case "base", "link", "meta":
			a.headStarted = true
			return false, true
		}
		if tt == xhtml.SelfClosingTagToken {
			return false, false
		}
		switch string(name) {
		case "html":
			if a.htmlSeen || a.headStarted {
				return false, false
			}
			a.htmlSeen = true
		case "head":
			if a.headStarted {
				return false, false
			}
			a.headStarted = true
		case "script", "style", "title":
			a.headStarted = true
			a.rawTag = string(name)
		case "body":
			return true, true
		default:
			return false, false
		}
		return false, true
	default:
		return false, false
	}
}

func (a *annotationInjector) passthrough() {
	// The current raw token is already pending. Drain read-ahead exactly once
	// before resuming the source, including on EOF or a token buffer limit.
	a.rest = io.MultiReader(bytes.NewReader(a.tokenizer.Buffered()), a.src)
	a.tokenizer = nil
}

func (a *annotationInjector) Close() error {
	return a.src.Close()
}

func annotationScriptTag(dashboardURL *url.URL) string {
	dashboard := *dashboardURL
	dashboard.Path = strings.TrimRight(dashboard.Path, "/") + "/annotator.js"
	dashboard.RawPath = ""
	// The script is not content-hashed, so the version keeps browsers
	// from reusing a copy cached from a previous deployment.
	dashboard.RawQuery = url.Values{"v": {buildinfo.Version()}}.Encode()
	dashboard.Fragment = ""

	src := dashboard.String()
	origin := dashboard.Scheme + "://" + dashboard.Host
	return `<script src="` + html.EscapeString(src) + `" data-coder-origin="` + html.EscapeString(origin) + `" defer></script>`
}
