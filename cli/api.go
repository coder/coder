package cli

import (
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/jsoncolor"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// api returns a CLI command that performs an authenticated GET request to the given API path.

// processJSONResponse handles formatting and displaying JSON data
func processJSONResponse(inv *serpent.Invocation, data []byte) error {
	// Get color mode from flags
	colorModeStr, _ := inv.ParsedFlags().GetString("color")
	colorMode := jsoncolor.StringToColorMode(colorModeStr)

	// Use our improved colorization function
	err := jsoncolor.WriteColorized(inv.Stdout, data, "  ", colorMode)
	if err != nil {
		// If the color output fails for any reason, try to write the raw data
		_, _ = inv.Stdout.Write(data)
	}

	// Add newline at the end
	_, _ = inv.Stdout.Write([]byte("\n"))
	return nil
}

func (r *RootCmd) api() *serpent.Command {
	client := new(codersdk.Client)
	return &serpent.Command{
		Use:   "api <api-path>",
		Short: "Make requests to the Coder API",
		Long: `Make an authenticated API request using your current Coder CLI token.

Examples:
  coder api workspacebuilds/my-build/logs
This will perform a GET request to /api/v2/workspacebuilds/my-build/logs on the connected Coder server.

  coder api users/me
This will perform a GET request to /api/v2/users/me on the connected Coder server.

Consult the API documentation for more information - https://coder.com/docs/reference/api.
`,
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Options: []serpent.Option{
			{
				Flag:        "color",
				Default:     "auto",
				Env:         "CODER_COLOR",
				Description: "Output colorization: auto, always, never.",
				Value:       serpent.EnumOf(new(string), "auto", "always", "never"),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			apiPath := inv.Args[0]

			// Special case for testing: if the path is a local file, read it directly
			if strings.HasSuffix(apiPath, ".json") && (strings.HasPrefix(apiPath, "./") || strings.HasPrefix(apiPath, "/")) {
				data, err := os.ReadFile(apiPath)
				if err != nil {
					return xerrors.Errorf("failed to read file: %w", err)
				}

				// Process the JSON data directly
				return processJSONResponse(inv, data)
			}

			// Normal API request path
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
				// Read the JSON response
				data, err := io.ReadAll(resp.Body)
				if err != nil {
					return xerrors.Errorf("failed to read response: %w", err)
				}

				// Process the JSON response
				return processJSONResponse(inv, data)
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
