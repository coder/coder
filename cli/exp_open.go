package cli

import (
	"fmt"
	"net/url"
	"strings"

	open "github.com/skratchdot/open-golang/open"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	appurl "github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/serpent"
)

func (r *RootCmd) expOpen() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "open",
		Short: "Open a workspace resource.",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.expOpenPort(),
		},
	}
	return cmd
}

func (r *RootCmd) expOpenPort() *serpent.Command {
	var (
		regionArg     string
		testOpenError bool
	)

	cmd := &serpent.Command{
		Use:   "port <workspace> <port>",
		Short: "Open a port on a workspace in the browser.",
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			ctx := inv.Context()

			if len(inv.Args) != 2 {
				return inv.Command.HelpHandler(inv)
			}

			workspaceName := inv.Args[0]
			portStr := inv.Args[1]

			if !appurl.PortRegex.MatchString(portStr) {
				return xerrors.Errorf("%q is not a valid port: must be 4-5 digits, optionally followed by 's' for HTTPS", portStr)
			}

			ws, agt, _, err := GetWorkspaceAndAgent(ctx, inv, client, false, workspaceName)
			if err != nil {
				return err
			}

			regions, err := client.Regions(ctx)
			if err != nil {
				return xerrors.Errorf("failed to fetch regions: %w", err)
			}

			preferredIdx := -1
			for i, reg := range regions {
				if reg.Name == regionArg {
					preferredIdx = i
					break
				}
			}
			if preferredIdx == -1 {
				allRegions := make([]string, len(regions))
				for i, reg := range regions {
					allRegions[i] = reg.Name
				}
				cliui.Errorf(inv.Stderr, "Preferred region %q not found!\nAvailable regions: %v", regionArg, allRegions)
				return xerrors.Errorf("region not found")
			}
			region := regions[preferredIdx]

			if region.WildcardHostname == "" {
				return xerrors.Errorf("wildcard access URL not set for region %q", region.Name)
			}

			baseURL, err := url.Parse(region.PathAppURL)
			if err != nil {
				return xerrors.Errorf("failed to parse proxy URL: %w", err)
			}

			appURL := appurl.ApplicationURL{
				AppSlugOrPort: portStr,
				AgentName:     agt.Name,
				WorkspaceName: ws.Name,
				Username:      ws.OwnerName,
			}
			openURL := baseURL.Scheme + "://" + strings.Replace(region.WildcardHostname, "*", appURL.String(), 1)

			insideAWorkspace := inv.Environ.Get("CODER") == "true"
			if insideAWorkspace {
				_, _ = fmt.Fprintf(inv.Stderr, "Please open the following URI on your local machine:\n\n")
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", openURL)
				return nil
			}
			_, _ = fmt.Fprintf(inv.Stderr, "Opening %s\n", openURL)

			if !testOpenError {
				err = open.Run(openURL)
			} else {
				err = xerrors.New("test.open-error: " + openURL)
			}
			return err
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "region",
			Env:         "CODER_OPEN_APP_REGION",
			Description: "Region to use when opening the port. By default, the primary Coder deployment is used.",
			Value:       serpent.StringOf(&regionArg),
			Default:     "primary",
		},
		{
			Flag:        "test.open-error",
			Description: "Don't run the open command.",
			Value:       serpent.BoolOf(&testOpenError),
			Hidden:      true,
		},
	}

	return cmd
}
