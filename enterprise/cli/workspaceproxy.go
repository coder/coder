package cli

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) workspaceProxy() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:     "workspace-proxy",
		Short:   "Manage workspace proxies",
		Aliases: []string{"wsproxy"},
		Hidden:  true,
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.proxyServer(),
			r.createProxy(),
			r.deleteProxy(),
			r.listProxies(),
		},
	}

	return cmd
}

func (r *RootCmd) deleteProxy() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "delete <name|id>",
		Short: "Delete a workspace proxy",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()
			err := client.DeleteWorkspaceProxyByName(ctx, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("delete workspace proxy %q: %w", inv.Args[0], err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Workspace proxy %q deleted successfully\n", inv.Args[0])
			return nil
		},
	}

	return cmd
}

func (r *RootCmd) createProxy() *clibase.Cmd {
	var (
		proxyName   string
		displayName string
		proxyIcon   string
		onlyToken   bool
		formatter   = cliui.NewOutputFormatter(
			// Text formatter should be human readable.
			cliui.ChangeFormatterData(cliui.TextFormat(), func(data any) (any, error) {
				response, ok := data.(codersdk.CreateWorkspaceProxyResponse)
				if !ok {
					return nil, xerrors.Errorf("unexpected type %T", data)
				}
				return fmt.Sprintf("Workspace Proxy %q created successfully. Save this token, it will not be shown again."+
					"\nToken: %s", response.Proxy.Name, response.ProxyToken), nil
			}),
			cliui.JSONFormat(),
			// Table formatter expects a slice, make a slice of one.
			cliui.ChangeFormatterData(cliui.TableFormat([]codersdk.CreateWorkspaceProxyResponse{}, []string{"proxy name", "proxy url", "proxy token"}),
				func(data any) (any, error) {
					response, ok := data.(codersdk.CreateWorkspaceProxyResponse)
					if !ok {
						return nil, xerrors.Errorf("unexpected type %T", data)
					}
					return []codersdk.CreateWorkspaceProxyResponse{response}, nil
				}),
		)
	)

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "create",
		Short: "Create a workspace proxy",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()
			if proxyName == "" {
				return xerrors.Errorf("proxy name is required")
			}

			resp, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
				Name:        proxyName,
				DisplayName: displayName,
				Icon:        proxyIcon,
			})
			if err != nil {
				return xerrors.Errorf("create workspace proxy: %w", err)
			}

			var output string
			if onlyToken {
				output = resp.ProxyToken
			} else {
				output, err = formatter.Format(ctx, resp)
				if err != nil {
					return err
				}
			}

			_, err = fmt.Fprintln(inv.Stdout, output)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	cmd.Options.Add(
		clibase.Option{
			Flag:        "name",
			Description: "Name of the proxy. This is used to identify the proxy.",
			Value:       clibase.StringOf(&proxyName),
		},
		clibase.Option{
			Flag:        "display-name",
			Description: "Display of the proxy. If omitted, the name is reused as the display name.",
			Value:       clibase.StringOf(&displayName),
		},
		clibase.Option{
			Flag:        "icon",
			Description: "Display icon of the proxy.",
			Value:       clibase.StringOf(&proxyIcon),
		},
		clibase.Option{
			Flag:        "only-token",
			Description: "Only print the token. This is useful for scripting.",
			Value:       clibase.BoolOf(&onlyToken),
		},
	)
	return cmd
}

func (r *RootCmd) listProxies() *clibase.Cmd {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]codersdk.WorkspaceProxy{}, []string{"name", "url", "proxy status"}),
		cliui.JSONFormat(),
		cliui.ChangeFormatterData(cliui.TextFormat(), func(data any) (any, error) {
			resp, ok := data.([]codersdk.WorkspaceProxy)
			if !ok {
				return nil, xerrors.Errorf("unexpected type %T", data)
			}
			var str strings.Builder
			_, _ = str.WriteString("Workspace Proxies:\n")
			sep := ""
			for i, proxy := range resp {
				_, _ = str.WriteString(sep)
				_, _ = str.WriteString(fmt.Sprintf("%d: %s %s %s", i, proxy.Name, proxy.URL, proxy.Status.Status))
				for _, errMsg := range proxy.Status.Report.Errors {
					_, _ = str.WriteString(color.RedString("\n\tErr: %s", errMsg))
				}
				for _, warnMsg := range proxy.Status.Report.Errors {
					_, _ = str.WriteString(color.YellowString("\n\tWarn: %s", warnMsg))
				}
				sep = "\n"
			}
			return str.String(), nil
		}),
	)

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List all workspace proxies",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()
			proxies, err := client.WorkspaceProxies(ctx)
			if err != nil {
				return xerrors.Errorf("list workspace proxies: %w", err)
			}

			output, err := formatter.Format(ctx, proxies)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, output)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
