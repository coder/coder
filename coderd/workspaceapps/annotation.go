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

// injectAnnotationScript rewrites a successful, uncompressed text/html GET
// response so that it loads the overlay script. The body is transformed as
// it streams, so server-rendered pages that flush progressively keep
// streaming and arbitrarily large pages are never buffered.
func injectAnnotationScript(resp *http.Response, scriptTag string) error {
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Encoding") != "" {
		return nil
	}
	if resp.Request != nil && resp.Request.Method != http.MethodGet {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "text/html") {
		return nil
	}

	tag := []byte(scriptTag)
	resp.Body = &annotationInjector{src: resp.Body, tag: tag}
	if resp.ContentLength >= 0 {
		resp.ContentLength += int64(len(tag))
		resp.Header.Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	// These describe the upstream bytes, which no longer match. Last-Modified
	// goes too: the rewritten page also depends on the Coder version in the
	// script URL, which a 304 against the upstream's date would preserve.
	for _, header := range []string{"ETag", "Last-Modified", "Content-MD5", "Digest", "Content-Digest", "Repr-Digest", "Accept-Ranges"} {
		resp.Header.Del(header)
	}
	return nil
}

// annotationMarkers are the closing tags before which the script may be
// inserted, in document order. With `defer` the position does not affect
// when the script runs, so the first one seen wins, which lets the body
// stream through with a six byte lookbehind. All have the same length.
var annotationMarkers = [][]byte{[]byte("</head>"), []byte("</body>"), []byte("</html>")}

const annotationMarkerLen = 7

// annotationInjector streams an HTML body and inserts the script tag exactly
// once: before the first marker it sees, or at the end of the body when no
// marker appears.
type annotationInjector struct {
	src io.ReadCloser
	tag []byte

	pending  []byte // Transformed bytes not yet handed to the caller.
	carry    []byte // Trailing bytes held back in case a marker straddles reads.
	injected bool
	eof      bool
	err      error // Source error, surfaced once pending has drained.
	buf      []byte
}

func (a *annotationInjector) Read(p []byte) (int, error) {
	for len(a.pending) == 0 {
		if a.err != nil {
			return 0, a.err
		}
		if a.eof {
			return 0, io.EOF
		}
		a.fill()
	}
	n := copy(p, a.pending)
	a.pending = a.pending[n:]
	return n, nil
}

// fill reads the next chunk from the source and moves as much as it safely
// can into pending. A source error is recorded rather than returned, so
// the bytes read alongside it still reach the caller first.
func (a *annotationInjector) fill() {
	if a.buf == nil {
		a.buf = make([]byte, 32<<10)
	}
	n, err := a.src.Read(a.buf)
	data := make([]byte, 0, len(a.carry)+n)
	data = append(data, a.carry...)
	data = append(data, a.buf[:n]...)
	a.carry = nil

	if err != nil && err != io.EOF {
		a.err = err
		a.pending = data
		return
	}
	a.eof = err == io.EOF

	if a.injected {
		a.pending = data
		return
	}
	if at := findAnnotationMarker(data); at >= 0 {
		a.pending = bytes.Join([][]byte{data[:at], a.tag, data[at:]}, nil)
		a.injected = true
		return
	}
	if a.eof {
		data = append(data, a.tag...)
		a.pending = data
		a.injected = true
		return
	}
	// Keep enough of the tail to recognize a marker split across reads.
	hold := annotationMarkerLen - 1
	if len(data) <= hold {
		a.carry = data
		return
	}
	a.pending = data[:len(data)-hold]
	a.carry = append([]byte(nil), data[len(data)-hold:]...)
}

func (a *annotationInjector) Close() error {
	return a.src.Close()
}

// findAnnotationMarker returns the index of the earliest marker in data or
// -1. Markers are ASCII, so folding is done per byte rather than with
// bytes.ToLower, which can change the length of non-ASCII text and shift
// every index after it.
func findAnnotationMarker(data []byte) int {
	for i := 0; i+annotationMarkerLen <= len(data); i++ {
		if data[i] != '<' || data[i+1] != '/' {
			continue
		}
		for _, marker := range annotationMarkers {
			if equalFoldASCII(data[i:i+annotationMarkerLen], marker) {
				return i
			}
		}
	}
	return -1
}

func equalFoldASCII(a, b []byte) bool {
	for i := range a {
		c := a[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != b[i] {
			return false
		}
	}
	return true
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
