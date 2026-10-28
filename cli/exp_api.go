package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// apiDefaultPageSize is the limit sent with --paginate when the caller
// does not set one.
const apiDefaultPageSize = 100

// apiCmd makes an authenticated request to an arbitrary Coder API path and
// writes the raw response body to stdout, similar to `gh api`.
func (r *RootCmd) apiCmd() *serpent.Command {
	var (
		method      string
		rawFields   []string
		typedFields []string
		headers     []string
		input       string
		include     bool
		paginate    bool
		jqExpr      string
	)
	cmd := &serpent.Command{
		Use:   "api <path>",
		Short: "Make an authenticated HTTP request to the Coder API and print the response.",
		Long: "The path is relative to /api/v2/, with or without a leading slash. " +
			"Paths that start with /api/ are used as given, for example /api/experimental/chats.\n\n" +
			"Fields are sent as query parameters for GET and HEAD requests or when --input is set, and as a JSON object otherwise. " +
			"The method defaults to GET, or POST when fields or --input are provided, so use -X GET to send fields as query parameters.\n\n" +
			"--field (-F) converts true, false, null, and integers to JSON values, and reads the value from a file with @path or from stdin with @-. " +
			"Use key[sub]=value for nested objects and key[]=value for arrays. --raw-field (-f) always sends strings.\n\n" +
			"--paginate follows offset and limit pagination until every page is printed. Each page is printed as its own JSON document. " +
			"Use the global --verbose flag to print request and response headers to stderr.\n\n" +
			FormatExamples(
				Example{
					Description: "Get the authenticated user",
					Command:     "coder exp api users/me",
				},
				Example{
					Description: "Print the username of every user",
					Command:     "coder exp api users --paginate --jq '.users[].username'",
				},
				Example{
					Description: "Create an organization",
					Command:     `coder exp api /organizations -f name=new-org -f display_name="New Organization"`,
				},
				Example{
					Description: "Send a JSON body from stdin",
					Command:     `echo '{"name":"new-name"}' | coder exp api -X PATCH workspaces/<id> --input -`,
				},
			),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			path, err := apiRequestPath(inv.Args[0])
			if err != nil {
				return err
			}
			fields, fieldsReadStdin, err := parseAPIFields(rawFields, typedFields, inv.Stdin)
			if err != nil {
				return err
			}
			if fieldsReadStdin && input == "-" {
				return xerrors.New("--input - and a @- field cannot both read from stdin")
			}
			reqHeaders, err := parseAPIHeaders(headers)
			if err != nil {
				return err
			}
			var jq *gojq.Code
			if jqExpr != "" {
				jq, err = compileJQ(jqExpr)
				if err != nil {
					return err
				}
			}

			method = strings.ToUpper(method)
			if method == "" {
				method = http.MethodGet
				if !paginate && (input != "" || len(fields) > 0) {
					method = http.MethodPost
				}
			}
			if paginate && method != http.MethodGet {
				return xerrors.Errorf("--paginate requires the GET method, got %s", method)
			}

			if apiURLIsInsecure(client.URL) {
				cliui.Warn(inv.Stderr, "The session token is sent over unencrypted HTTP to "+client.URL.Host+".",
					"Use an https:// URL for the deployment to protect the token.")
			}

			query := url.Values{}
			var body any
			switch {
			case input != "" || method == http.MethodGet || method == http.MethodHead:
				for _, f := range fields {
					query.Add(f.key, apiFieldString(f.value))
				}
			case len(fields) > 0:
				body, err = buildAPIBody(fields)
				if err != nil {
					return err
				}
			}
			switch {
			case input == "-":
				body = inv.Stdin
			case input != "":
				f, err := os.Open(input)
				if err != nil {
					return xerrors.Errorf("open input: %w", err)
				}
				defer f.Close()
				body = f
			}

			req := apiRequest{
				inv:     inv,
				client:  client,
				method:  method,
				path:    path,
				body:    body,
				headers: reqHeaders,
				include: include,
				verbose: r.verbose,
				jq:      jq,
			}
			if paginate {
				return req.paginate(query)
			}
			resp, err := req.do(query)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= http.StatusBadRequest {
				return req.writeError(resp)
			}
			return req.writeBody(resp.Body)
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:          "method",
			FlagShorthand: "X",
			Description:   "The HTTP method for the request.",
			Value:         serpent.StringOf(&method),
		},
		{
			Flag:          "raw-field",
			FlagShorthand: "f",
			Description:   "Add a string parameter in key=value format. Can be specified multiple times.",
			Value:         serpent.StringArrayOf(&rawFields),
		},
		{
			Flag:          "field",
			FlagShorthand: "F",
			Description:   "Add a typed parameter in key=value format. Can be specified multiple times.",
			Value:         serpent.StringArrayOf(&typedFields),
		},
		{
			// The root command already owns --header for headers sent on
			// every request, so this flag uses a distinct name.
			Flag:          "request-header",
			FlagShorthand: "H",
			Description:   "Add a request header in key:value format. Can be specified multiple times.",
			Value:         serpent.StringArrayOf(&headers),
		},
		{
			Flag:        "input",
			Description: "The file to use as the request body, or - to read from stdin.",
			Value:       serpent.StringOf(&input),
		},
		{
			Flag:          "include",
			FlagShorthand: "i",
			Description:   "Print the response status line and headers before the body.",
			Value:         serpent.BoolOf(&include),
		},
		{
			Flag:        "paginate",
			Description: "Request every page of results by following offset and limit pagination.",
			Value:       serpent.BoolOf(&paginate),
		},
		{
			Flag:          "jq",
			FlagShorthand: "q",
			Description:   "Filter each JSON response with a jq expression. String results are printed without quotes.",
			Value:         serpent.StringOf(&jqExpr),
		},
	}
	return cmd
}

type apiRequest struct {
	inv     *serpent.Invocation
	client  *codersdk.Client
	method  string
	path    string
	body    any
	headers http.Header
	include bool
	verbose bool
	jq      *gojq.Code
}

func (a apiRequest) do(query url.Values) (*http.Response, error) {
	opts := []codersdk.RequestOption{func(req *http.Request) {
		if len(query) > 0 {
			q := req.URL.Query()
			for k, vs := range query {
				for _, v := range vs {
					q.Add(k, v)
				}
			}
			req.URL.RawQuery = q.Encode()
		}
		for k, vs := range a.headers {
			req.Header[k] = vs
		}
	}}
	resp, err := a.client.Request(a.inv.Context(), a.method, a.path, a.body, opts...)
	if err != nil {
		return nil, xerrors.Errorf("%s %s: %w", a.method, a.path, err)
	}
	if a.verbose {
		writeAPIVerbose(a.inv.Stderr, resp)
	}
	if a.include {
		if err := writeAPIResponseHead(a.inv.Stdout, resp); err != nil {
			_ = resp.Body.Close()
			return nil, err
		}
	}
	return resp, nil
}

// writeError prints the error body unmodified so scripts can inspect it, then
// returns a friendly error built from the Coder error response. It closes
// the response body.
func (a apiRequest) writeError(resp *http.Response) error {
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return xerrors.Errorf("read response body: %w", err)
	}
	if _, err := a.inv.Stdout.Write(raw); err != nil {
		return xerrors.Errorf("write response body: %w", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	apiErr := codersdk.ReadBodyAsError(resp)
	if sdkErr, ok := codersdk.AsError(apiErr); ok && resp.StatusCode == http.StatusNotFound && sdkErr.Message == "Route not found." {
		sdkErr.Helper = "Check the path against the API reference at https://coder.com/docs/reference/api."
	}
	return apiErr
}

// writeBody copies a successful response body to stdout, applying the jq
// filter when one is set.
func (a apiRequest) writeBody(body io.Reader) error {
	if a.jq == nil {
		if _, err := io.Copy(a.inv.Stdout, body); err != nil {
			return xerrors.Errorf("read response body: %w", err)
		}
		return nil
	}
	dec := json.NewDecoder(body)
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return xerrors.Errorf("--jq requires a JSON response: %w", err)
	}
	iter := a.jq.Run(v)
	for {
		out, ok := iter.Next()
		if !ok {
			return nil
		}
		if err, ok := out.(error); ok {
			return xerrors.Errorf("jq: %w", err)
		}
		var line []byte
		if s, ok := out.(string); ok {
			line = []byte(s)
		} else {
			var err error
			line, err = gojq.Marshal(out)
			if err != nil {
				return xerrors.Errorf("jq: encode result: %w", err)
			}
		}
		if _, err := fmt.Fprintf(a.inv.Stdout, "%s\n", line); err != nil {
			return xerrors.Errorf("write output: %w", err)
		}
	}
}

// paginate requests pages with increasing offsets until a page is shorter
// than the limit, the reported total is reached, or a page is empty. Most
// Coder list endpoints share offset and limit handling, but their response
// shapes differ, so the page size is read from either a top-level array or
// the single array field of an object such as {"users": [...], "count": 3}.
func (a apiRequest) paginate(query url.Values) error {
	limit := apiDefaultPageSize
	if v := query.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return xerrors.Errorf("--paginate requires a positive limit, got %q", v)
		}
		limit = n
	}
	offset := 0
	if v := query.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return xerrors.Errorf("--paginate requires a non-negative offset, got %q", v)
		}
		offset = n
	}

	var previous []byte
	for page := 0; ; page++ {
		pageQuery := url.Values{}
		for k, vs := range query {
			pageQuery[k] = slices.Clone(vs)
		}
		pageQuery.Set("limit", strconv.Itoa(limit))
		pageQuery.Set("offset", strconv.Itoa(offset))

		resp, err := a.do(pageQuery)
		if err != nil {
			return err
		}
		if resp.StatusCode >= http.StatusBadRequest {
			return a.writeError(resp)
		}
		raw, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return xerrors.Errorf("read response body: %w", err)
		}
		if page > 0 && bytes.Equal(raw, previous) {
			return xerrors.Errorf("%s did not change between offsets; the endpoint might not support pagination", a.path)
		}
		previous = raw

		items, total, ok := apiPageSize(raw)
		if page == 0 || items > 0 {
			if err := a.writeBody(bytes.NewReader(raw)); err != nil {
				return err
			}
		}
		if !ok {
			if page == 0 {
				cliui.Warn(a.inv.Stderr, "The response has no list of results, so only one page was requested.")
			}
			return nil
		}
		offset += items
		if items == 0 || items < limit || (total >= 0 && offset >= total) {
			return nil
		}
	}
}

// apiPageSize reports how many results a page holds and the total reported
// by a "count" field, or -1 when there is none. ok is false when the body
// has no recognizable list of results.
func apiPageSize(raw []byte) (items int, total int, ok bool) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, -1, false
	}
	switch t := v.(type) {
	case []any:
		return len(t), -1, true
	case map[string]any:
		var lists [][]any
		for _, field := range t {
			if list, isList := field.([]any); isList {
				lists = append(lists, list)
			}
		}
		if len(lists) != 1 {
			return 0, -1, false
		}
		total = -1
		if count, isNumber := t["count"].(float64); isNumber {
			total = int(count)
		}
		return len(lists[0]), total, true
	default:
		return 0, -1, false
	}
}

func compileJQ(expr string) (*gojq.Code, error) {
	q, err := gojq.Parse(expr)
	if err != nil {
		return nil, xerrors.Errorf("parse --jq expression: %w", err)
	}
	code, err := gojq.Compile(q)
	if err != nil {
		return nil, xerrors.Errorf("compile --jq expression: %w", err)
	}
	return code, nil
}

// apiRequestPath turns a user-supplied path into a path on the configured
// deployment. Absolute URLs are rejected so the session token is never sent
// to another host.
func apiRequestPath(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", xerrors.Errorf("parse path: %w", err)
	}
	if u.Scheme != "" || u.Host != "" {
		return "", xerrors.Errorf("path %q must not include a scheme or host; requests are always sent to the configured Coder URL", raw)
	}
	if strings.HasPrefix(raw, "/api/") {
		return raw, nil
	}
	return "/api/v2/" + strings.TrimPrefix(raw, "/"), nil
}

// apiURLIsInsecure reports whether requests to u send the session token in
// clear text over a network. Loopback addresses are exempt so local
// development deployments do not warn.
func apiURLIsInsecure(u *url.URL) bool {
	if u == nil || u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}

type apiField struct {
	key   string
	value any
}

// parseAPIFields parses --raw-field values as strings and --field values as
// typed values. It reports whether any value was read from stdin.
func parseAPIFields(raw, typed []string, stdin io.Reader) ([]apiField, bool, error) {
	fields := make([]apiField, 0, len(raw)+len(typed))
	for _, f := range raw {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, false, xerrors.Errorf("invalid field %q: expected key=value", f)
		}
		fields = append(fields, apiField{key: k, value: v})
	}
	readStdin := false
	for _, f := range typed {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, false, xerrors.Errorf("invalid field %q: expected key=value", f)
		}
		if v == "@-" {
			if readStdin {
				return nil, false, xerrors.New("only one field can read from stdin")
			}
			readStdin = true
		}
		value, err := parseAPITypedValue(v, stdin)
		if err != nil {
			return nil, false, xerrors.Errorf("field %q: %w", k, err)
		}
		fields = append(fields, apiField{key: k, value: value})
	}
	return fields, readStdin, nil
}

// parseAPITypedValue converts a --field value using the same rules as
// `gh api`: true, false, null, and integers become JSON values, and @path or
// @- reads the value from a file or stdin unmodified.
func parseAPITypedValue(v string, stdin io.Reader) (any, error) {
	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil //nolint:nilnil // nil is the JSON null value, not a missing result.
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n, nil
	}
	if name, ok := strings.CutPrefix(v, "@"); ok {
		var (
			b   []byte
			err error
		)
		if name == "-" {
			b, err = io.ReadAll(stdin)
		} else {
			b, err = os.ReadFile(name)
		}
		if err != nil {
			return nil, xerrors.Errorf("read value: %w", err)
		}
		return string(b), nil
	}
	return v, nil
}

func apiFieldString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// buildAPIBody builds a JSON object from fields, expanding key[sub] into
// nested objects and a trailing key[] into arrays.
func buildAPIBody(fields []apiField) (map[string]any, error) {
	body := map[string]any{}
	for _, f := range fields {
		path, err := parseAPIFieldKey(f.key)
		if err != nil {
			return nil, err
		}
		if err := setAPIBodyValue(body, path, f.value); err != nil {
			return nil, xerrors.Errorf("field %q: %w", f.key, err)
		}
	}
	return body, nil
}

// parseAPIFieldKey splits a key such as a[b][] into ["a", "b", ""], where an
// empty segment means "append to an array".
func parseAPIFieldKey(key string) ([]string, error) {
	name, rest, _ := strings.Cut(key, "[")
	if name == "" {
		return nil, xerrors.Errorf("invalid field key %q", key)
	}
	path := []string{name}
	if rest == "" && !strings.Contains(key, "[") {
		return path, nil
	}
	rest = "[" + rest
	for rest != "" {
		if rest[0] != '[' {
			return nil, xerrors.Errorf("invalid field key %q", key)
		}
		end := strings.IndexByte(rest, ']')
		if end < 0 {
			return nil, xerrors.Errorf("invalid field key %q: missing ]", key)
		}
		path = append(path, rest[1:end])
		rest = rest[end+1:]
	}
	for i, seg := range path[1:] {
		if seg == "" && i != len(path)-2 {
			return nil, xerrors.Errorf("invalid field key %q: [] is only supported at the end", key)
		}
	}
	return path, nil
}

func setAPIBodyValue(obj map[string]any, path []string, value any) error {
	key := path[0]
	switch {
	case len(path) == 1:
		if _, isObject := obj[key].(map[string]any); isObject {
			return xerrors.Errorf("%q is already an object", key)
		}
		obj[key] = value
		return nil
	case len(path) == 2 && path[1] == "":
		existing, present := obj[key]
		list, isList := existing.([]any)
		if present && !isList {
			return xerrors.Errorf("%q is already set to a non-array value", key)
		}
		obj[key] = append(list, value)
		return nil
	default:
		existing, present := obj[key]
		child, isObject := existing.(map[string]any)
		if present && !isObject {
			return xerrors.Errorf("%q is already set to a non-object value", key)
		}
		if !present {
			child = map[string]any{}
			obj[key] = child
		}
		return setAPIBodyValue(child, path[1:], value)
	}
}

func parseAPIHeaders(headers []string) (http.Header, error) {
	h := http.Header{}
	for _, raw := range headers {
		k, v, ok := strings.Cut(raw, ":")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, xerrors.Errorf("invalid header %q: expected key:value", raw)
		}
		// TrimSpace also drops a trailing \r from values copied out of
		// CRLF-terminated files.
		h.Add(k, strings.TrimSpace(v))
	}
	return h, nil
}

func writeAPIResponseHead(w io.Writer, resp *http.Response) error {
	if _, err := fmt.Fprintf(w, "%s %s\n", resp.Proto, resp.Status); err != nil {
		return xerrors.Errorf("write status line: %w", err)
	}
	if err := resp.Header.Write(w); err != nil {
		return xerrors.Errorf("write response headers: %w", err)
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return xerrors.Errorf("write header separator: %w", err)
	}
	return nil
}

// apiRedactedHeaders are never printed by --verbose.
var apiRedactedHeaders = []string{
	codersdk.SessionTokenHeader,
	"Authorization",
	"Cookie",
	"Proxy-Authorization",
	"Set-Cookie",
}

// writeAPIVerbose prints the request and response lines and headers in the
// style of curl -v. Write errors are ignored because this is diagnostic
// output on stderr.
func writeAPIVerbose(w io.Writer, resp *http.Response) {
	redact := func(h http.Header) http.Header {
		h = h.Clone()
		for _, name := range apiRedactedHeaders {
			if h.Get(name) != "" {
				h.Set(name, "REDACTED")
			}
		}
		return h
	}
	writeHeaders := func(prefix string, h http.Header) {
		keys := make([]string, 0, len(h))
		for k := range h {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			for _, v := range h[k] {
				_, _ = fmt.Fprintf(w, "%s %s: %s\n", prefix, k, v)
			}
		}
	}
	if req := resp.Request; req != nil {
		_, _ = fmt.Fprintf(w, "> %s %s\n", req.Method, req.URL.String())
		writeHeaders(">", redact(req.Header))
		_, _ = fmt.Fprintln(w, ">")
	}
	_, _ = fmt.Fprintf(w, "< %s %s\n", resp.Proto, resp.Status)
	writeHeaders("<", redact(resp.Header))
	_, _ = fmt.Fprintln(w, "<")
}
