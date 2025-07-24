package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// read returns a CLI command that performs an authenticated GET request to the given API path.
func (r *RootCmd) read() *serpent.Command {
	client := new(codersdk.Client)
	return &serpent.Command{
		Use:   "read <api-path>",
		Short: "Read an authenticated API endpoint using your current Coder CLI token",
		Long: `Read an authenticated API endpoint using your current Coder CLI token.

Example:
  coder read workspacebuilds/my-build/logs
This will perform a GET request to /api/v2/workspacebuilds/my-build/logs on the connected Coder server.
`,
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			apiPath := inv.Args[0]
			if !strings.HasPrefix(apiPath, "/") {
				apiPath = "/api/v2/" + apiPath
			}
			resp, err := client.Request(inv.Context(), http.MethodGet, apiPath, nil)
			if err != nil {
				return xerrors.Errorf("request failed: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				return xerrors.Errorf("API error: %s\n%s", resp.Status, string(body))
			}

			contentType := resp.Header.Get("Content-Type")
			if strings.HasPrefix(contentType, "application/json") {
				// Pretty-print JSON
				var raw interface{}
				data, err := io.ReadAll(resp.Body)
				if err != nil {
					return xerrors.Errorf("failed to read response: %w", err)
				}
				err = json.Unmarshal(data, &raw)
				if err == nil {
					pretty, err := json.MarshalIndent(raw, "", "  ")
					if err == nil {
						_, err = inv.Stdout.Write(pretty)
						if err != nil {
							return xerrors.Errorf("failed to write output: %w", err)
						}
						_, _ = inv.Stdout.Write([]byte("\n"))
						return nil
					}
				}
				// If JSON formatting fails, fall back to raw output
				_, _ = inv.Stdout.Write(data)
				_, _ = inv.Stdout.Write([]byte("\n"))
				return nil
			}
			// Non-JSON: stream as before
			_, err = io.Copy(inv.Stdout, resp.Body)
			if err != nil {
				return xerrors.Errorf("failed to read response: %w", err)
			}
			return nil
		},
	}
}
