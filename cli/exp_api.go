package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// apiCmd makes an authenticated request to an arbitrary Coder API path and
// writes the raw response body to stdout, similar to `gh api`.
func (r *RootCmd) apiCmd() *serpent.Command {
	var (
		method  string
		fields  []string
		headers []string
		input   string
		include bool
	)
	cmd := &serpent.Command{
		Use:   "api <path>",
		Short: "Make an authenticated HTTP request to the Coder API and print the response.",
		Long: "The path is relative to /api/v2/ unless it starts with a slash. " +
			"Fields are sent as query parameters for GET and HEAD requests and as a JSON object otherwise. " +
			"The method defaults to GET, or POST when fields or --input are provided, so use -X GET to send fields as query parameters.\n\n" +
			FormatExamples(
				Example{
					Description: "Get the authenticated user",
					Command:     "coder exp api users/me",
				},
				Example{
					Description: "List workspaces with a query parameter",
					Command:     "coder exp api -X GET workspaces -f q=owner:me",
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
			if input != "" && len(fields) > 0 {
				return xerrors.New("--input and --field cannot be used together")
			}

			fieldValues, err := parseAPIFields(fields)
			if err != nil {
				return err
			}
			reqHeaders, err := parseAPIHeaders(headers)
			if err != nil {
				return err
			}

			method = strings.ToUpper(method)
			if method == "" {
				method = http.MethodGet
				if input != "" || len(fields) > 0 {
					method = http.MethodPost
				}
			}

			var (
				body any
				opts []codersdk.RequestOption
			)
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
			case len(fieldValues) > 0 && (method == http.MethodGet || method == http.MethodHead):
				for k, vs := range fieldValues {
					for _, v := range vs {
						opts = append(opts, codersdk.WithQueryParam(k, v))
					}
				}
			case len(fieldValues) > 0:
				obj := make(map[string]string, len(fieldValues))
				for k, vs := range fieldValues {
					obj[k] = vs[len(vs)-1]
				}
				body = obj
			}
			opts = append(opts, func(req *http.Request) {
				for k, vs := range reqHeaders {
					req.Header[k] = vs
				}
			})

			resp, err := client.Request(inv.Context(), method, path, body, opts...)
			if err != nil {
				return xerrors.Errorf("%s %s: %w", method, path, err)
			}
			defer resp.Body.Close()

			if include {
				if _, err := fmt.Fprintf(inv.Stdout, "%s %s\n", resp.Proto, resp.Status); err != nil {
					return xerrors.Errorf("write status line: %w", err)
				}
				if err := resp.Header.Write(inv.Stdout); err != nil {
					return xerrors.Errorf("write response headers: %w", err)
				}
				if _, err := fmt.Fprintln(inv.Stdout); err != nil {
					return xerrors.Errorf("write header separator: %w", err)
				}
			}
			if _, err := io.Copy(inv.Stdout, resp.Body); err != nil {
				return xerrors.Errorf("read response body: %w", err)
			}
			if resp.StatusCode >= http.StatusBadRequest {
				return xerrors.Errorf("%s %s: %s", method, path, resp.Status)
			}
			return nil
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
			Flag:          "field",
			FlagShorthand: "f",
			Description:   "Add a string parameter in key=value format. Can be specified multiple times.",
			Value:         serpent.StringArrayOf(&fields),
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
	}
	return cmd
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
	if strings.HasPrefix(raw, "/") {
		return raw, nil
	}
	return "/api/v2/" + raw, nil
}

func parseAPIFields(fields []string) (url.Values, error) {
	values := url.Values{}
	for _, f := range fields {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, xerrors.Errorf("invalid field %q: expected key=value", f)
		}
		values.Add(k, v)
	}
	return values, nil
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
