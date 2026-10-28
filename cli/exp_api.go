package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// apiMethods are the HTTP methods accepted as the optional first argument of
// `coder exp api`. Any other first argument is treated as the path.
var apiMethods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodOptions,
}

// apiCmd makes an authenticated request to an arbitrary Coder API path and
// writes the raw response body to stdout. Request items follow httpie syntax.
func (r *RootCmd) apiCmd() *serpent.Command {
	var input string
	cmd := &serpent.Command{
		Use:   "api [METHOD] <path> [REQUEST_ITEM ...]",
		Short: "Make an authenticated HTTP request to the Coder API and print the response.",
		Long: "The path is relative to /api/v2/, with or without a leading slash. " +
			"Paths that start with /api/ are used as given, for example /api/experimental/chats.\n\n" +
			"METHOD is optional and defaults to GET, or POST when the request has data items or --input.\n\n" +
			"Request items use httpie syntax:\n" +
			"  Name:value      request header\n" +
			"  name==value     query parameter\n" +
			"  name=value      JSON string field\n" +
			"  name:=json      raw JSON field: number, bool, null, array, or object\n" +
			"  name=@file      JSON string field read from a file, or - for stdin\n" +
			"  name:=@file     raw JSON field read from a file, or - for stdin\n\n" +
			"Data items build a JSON object body. Escape a separator character in a key with a backslash, " +
			"for example foo\\==bar. Use --input to send a raw body instead of data items.\n\n" +
			FormatExamples(
				Example{
					Description: "Get the authenticated user",
					Command:     "coder exp api users/me",
				},
				Example{
					Description: "List users with a search query",
					Command:     "coder exp api users q==status:active limit==10",
				},
				Example{
					Description: "Create a user in an organization",
					Command:     `coder exp api users username=alice email=alice@example.com password=Secure-Password1 'organization_ids:=["<org-id>"]'`,
				},
				Example{
					Description: "Send a JSON body from stdin",
					Command:     `echo '{"name":"new-name"}' | coder exp api PATCH workspaces/<id> --input -`,
				},
			),
		Middleware: serpent.RequireRangeArgs(1, -1),
		Handler: func(inv *serpent.Invocation) error {
			method, rawPath, rawItems := splitAPIArgs(inv.Args)
			if rawPath == "" {
				return xerrors.Errorf("missing path after method %s", method)
			}
			path, err := apiRequestPath(rawPath)
			if err != nil {
				return err
			}
			items, itemsReadStdin, err := parseAPIRequestItems(rawItems, inv.Stdin)
			if err != nil {
				return err
			}
			if itemsReadStdin && input == "-" {
				return xerrors.New("--input - and a request item cannot both read stdin")
			}
			if input != "" && len(items.data) > 0 {
				return xerrors.New("--input cannot be combined with data items")
			}
			if method == "" {
				method = http.MethodGet
				if input != "" || len(items.data) > 0 {
					method = http.MethodPost
				}
			}

			var body any
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
			case len(items.data) > 0:
				body = items.data
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			if apiURLIsInsecure(client.URL) {
				cliui.Warn(inv.Stderr, "The session token is sent over unencrypted HTTP to "+client.URL.Host+".",
					"Use an https:// URL for the deployment to protect the token.")
			}

			resp, err := client.Request(inv.Context(), method, path, body, func(req *http.Request) {
				// Append to the raw query so a query already in the path is
				// sent exactly as written.
				if encoded := items.query.Encode(); encoded != "" {
					if req.URL.RawQuery != "" {
						req.URL.RawQuery += "&"
					}
					req.URL.RawQuery += encoded
				}
				for k, vs := range items.headers {
					req.Header[k] = vs
				}
			})
			if err != nil {
				return xerrors.Errorf("%s %s: %w", method, path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode >= http.StatusBadRequest {
				return writeAPIError(inv.Stdout, resp)
			}
			if _, err := io.Copy(inv.Stdout, resp.Body); err != nil {
				return xerrors.Errorf("read response body: %w", err)
			}
			return nil
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:        "input",
			Description: "The file to use as the raw request body, or - to read from stdin. Cannot be combined with data items.",
			Value:       serpent.StringOf(&input),
		},
	}
	return cmd
}

// splitAPIArgs separates the optional method, the path, and the request
// items. The first argument is a method only when it matches a known HTTP
// method, so paths such as "users" are never mistaken for one. The returned
// path is empty when only a method was given.
func splitAPIArgs(args []string) (method, path string, items []string) {
	first := strings.ToUpper(args[0])
	for _, m := range apiMethods {
		if first == m {
			if len(args) < 2 {
				return m, "", nil
			}
			return m, args[1], args[2:]
		}
	}
	return "", args[0], args[1:]
}

// writeAPIError prints the error body unmodified so scripts can inspect it,
// then returns the Coder error built from the response.
func writeAPIError(w io.Writer, resp *http.Response) error {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return xerrors.Errorf("read response body: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return xerrors.Errorf("write response body: %w", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	apiErr := codersdk.ReadBodyAsError(resp)
	if sdkErr, ok := codersdk.AsError(apiErr); ok && resp.StatusCode == http.StatusNotFound && sdkErr.Message == "Route not found." {
		sdkErr.Helper = "Check the path against the API reference at https://coder.com/docs/reference/api."
	}
	return apiErr
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

// apiItemSeparators lists request item separators from longest to shortest,
// so the longest separator wins when several match at the same position.
var apiItemSeparators = []string{":=@", "=@", "==", ":=", "=", ":"}

// apiEscapable holds the characters a backslash escapes in a request item.
// A backslash before any other character is kept literally, as in httpie.
const apiEscapable = ":=@"

type apiRequestItems struct {
	headers http.Header
	query   url.Values
	data    map[string]any
}

// parseAPIRequestItems parses httpie-style request items. It reports whether
// an item read its value from stdin.
func parseAPIRequestItems(raw []string, stdin io.Reader) (apiRequestItems, bool, error) {
	items := apiRequestItems{
		headers: http.Header{},
		query:   url.Values{},
		data:    map[string]any{},
	}
	readStdin := false
	for _, arg := range raw {
		key, sep, value, ok := splitAPIRequestItem(arg)
		if !ok {
			return items, false, xerrors.Errorf("invalid request item %q: expected Name:value, name==value, name=value, name:=json, name=@file, or name:=@file", arg)
		}
		if key == "" {
			return items, false, xerrors.Errorf("invalid request item %q: missing name", arg)
		}
		switch sep {
		case ":":
			items.headers.Add(key, strings.TrimSpace(value))
			continue
		case "==":
			items.query.Add(key, value)
			continue
		}

		if _, exists := items.data[key]; exists {
			return items, false, xerrors.Errorf("duplicate data field %q", key)
		}
		if sep == "=@" || sep == ":=@" {
			if value == "-" {
				if readStdin {
					return items, false, xerrors.Errorf("request item %q: only one request item can read stdin", arg)
				}
				readStdin = true
			}
			b, err := readAPIItemFile(value, stdin)
			if err != nil {
				return items, false, xerrors.Errorf("request item %q: %w", arg, err)
			}
			value = string(b)
		}
		switch sep {
		case "=", "=@":
			items.data[key] = value
		case ":=", ":=@":
			if !json.Valid([]byte(value)) {
				return items, false, xerrors.Errorf("request item %q: value is not valid JSON", arg)
			}
			items.data[key] = json.RawMessage(value)
		}
	}
	return items, readStdin, nil
}

func readAPIItemFile(name string, stdin io.Reader) ([]byte, error) {
	if name == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, xerrors.Errorf("read stdin: %w", err)
		}
		return b, nil
	}
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, xerrors.Errorf("read file: %w", err)
	}
	return b, nil
}

// splitAPIRequestItem finds the first unescaped position where a separator
// matches, takes the longest separator at that position, and returns the
// unescaped key and value around it.
func splitAPIRequestItem(arg string) (key, sep, value string, ok bool) {
	type char struct {
		r       rune
		escaped bool
	}
	runes := []rune(arg)
	chars := make([]char, 0, len(runes))
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' && i+1 < len(runes) && strings.ContainsRune(apiEscapable, runes[i+1]) {
			chars = append(chars, char{r: runes[i+1], escaped: true})
			i++
			continue
		}
		chars = append(chars, char{r: runes[i]})
	}
	join := func(cs []char) string {
		rs := make([]rune, len(cs))
		for i, c := range cs {
			rs[i] = c.r
		}
		return string(rs)
	}
	matches := func(at int, s string) bool {
		for j, r := range []rune(s) {
			if at+j >= len(chars) || chars[at+j].escaped || chars[at+j].r != r {
				return false
			}
		}
		return true
	}
	for i := range chars {
		for _, s := range apiItemSeparators {
			if matches(i, s) {
				return join(chars[:i]), s, join(chars[i+len([]rune(s)):]), true
			}
		}
	}
	return "", "", "", false
}
