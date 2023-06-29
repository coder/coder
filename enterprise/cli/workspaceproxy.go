package cli

import (
	"context"
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
		Use:   "workspace-proxy",
		Short: "Workspace proxies provide low-latency experiences for geo-distributed teams.",
		Long: "Workspace proxies provide low-latency experiences for geo-distributed teams. " +
			"It will act as a connection gateway to your workspace providing a lower latency solution " +
			"to connecting to your workspace if Coder and your workspace are deployed in different regions.",
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
			r.patchProxy(),
			r.regenerateProxyToken(),
		},
	}

	return cmd
}

func (r *RootCmd) regenerateProxyToken() *clibase.Cmd {
	formatter := newUpdateProxyResponseFormatter()
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use: "regenerate-token <name|id>",
		Short: "Regenerate a workspace proxy authentication token. " +
			"This will invalidate the existing authentication token.",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()
			// This is cheeky, but you can also use a uuid string in
			// 'DeleteWorkspaceProxyByName' and it will work.
			proxy, err := client.WorkspaceProxyByName(ctx, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("fetch workspace proxy %q: %w", inv.Args[0], err)
			}

			// Only regenerate the token
			updated, err := client.PatchWorkspaceProxy(ctx, codersdk.PatchWorkspaceProxy{
				ID:              proxy.ID,
				Name:            proxy.Name,
				DisplayName:     proxy.DisplayName,
				Icon:            proxy.Icon,
				RegenerateToken: true,
			})
			if err != nil {
				return xerrors.Errorf("update workspace proxy %q: %w", inv.Args[0], err)
			}

			output, err := formatter.Format(ctx, updated)
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

func (r *RootCmd) patchProxy() *clibase.Cmd {
	var (
		proxyName   string
		displayName string
		proxyIcon   string
		formatter   = cliui.NewOutputFormatter(
			// Text formatter should be human readable.
			cliui.ChangeFormatterData(cliui.TextFormat(), func(data any) (any, error) {
				response, ok := data.(codersdk.WorkspaceProxy)
				if !ok {
					return nil, xerrors.Errorf("unexpected type %T", data)
				}
				return fmt.Sprintf("Workspace Proxy %q updated successfully.", response.Name), nil
			}),
			cliui.JSONFormat(),
			// Table formatter expects a slice, make a slice of one.
			cliui.ChangeFormatterData(cliui.TableFormat([]codersdk.WorkspaceProxy{}, []string{"proxy name", "proxy url"}),
				func(data any) (any, error) {
					response, ok := data.(codersdk.WorkspaceProxy)
					if !ok {
						return nil, xerrors.Errorf("unexpected type %T", data)
					}
					return []codersdk.WorkspaceProxy{response}, nil
				}),
		)
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "edit <name|id>",
		Short: "Edit a workspace proxy",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()
			if proxyIcon == "" && displayName == "" && proxyName == "" {
				return xerrors.Errorf("specify at least one field to update")
			}

			// This is cheeky, but you can also use a uuid string in
			// 'DeleteWorkspaceProxyByName' and it will work.
			proxy, err := client.WorkspaceProxyByName(ctx, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("fetch workspace proxy %q: %w", inv.Args[0], err)
			}

			// Use the existing values if the user didn't specify them.
			if proxyName == "" {
				proxyName = proxy.Name
			}
			if displayName == "" {
				displayName = proxy.DisplayName
			}
			if proxyIcon == "" {
				proxyIcon = proxy.Icon
			}

			updated, err := client.PatchWorkspaceProxy(ctx, codersdk.PatchWorkspaceProxy{
				ID:          proxy.ID,
				Name:        proxyName,
				DisplayName: displayName,
				Icon:        proxyIcon,
			})
			if err != nil {
				return xerrors.Errorf("update workspace proxy %q: %w", inv.Args[0], err)
			}

			output, err := formatter.Format(ctx, updated.Proxy)
			if err != nil {
				return xerrors.Errorf("format response: %w", err)
			}
			_, err = fmt.Fprintln(inv.Stdout, output)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	cmd.Options.Add(
		clibase.Option{
			Flag:        "name",
			Description: "(Optional) Name of the proxy. This is used to identify the proxy.",
			Value:       clibase.StringOf(&proxyName),
		},
		clibase.Option{
			Flag:        "display-name",
			Description: "(Optional) Display of the proxy. A more human friendly name to be displayed.",
			Value:       clibase.StringOf(&displayName),
		},
		clibase.Option{
			Flag:        "icon",
			Description: "(Optional) Display icon of the proxy.",
			Value:       clibase.StringOf(&proxyIcon),
		},
	)

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
		noPrompts   bool
		formatter   = newUpdateProxyResponseFormatter()
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
			var err error
			if proxyName == "" && !noPrompts {
				proxyName, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Proxy Name:",
				})
				if err != nil {
					return err
				}
			}
			if displayName == "" && !noPrompts {
				displayName, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text:    "Display Name:",
					Default: proxyName,
				})
				if err != nil {
					return err
				}
			}

			if proxyIcon == "" && !noPrompts {
				proxyIcon, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text:    "Icon URL:",
					Default: "/emojis/1f5fa.png",
					Validate: func(s string) error {
						if !(strings.HasPrefix(s, "/emojis/") || strings.HasPrefix(s, "http")) {
							return xerrors.New("icon must be a relative path to an emoji or a publicly hosted image URL")
						}
						return nil
					},
				})
				if err != nil {
					return err
				}
			}

			if proxyName == "" {
				return xerrors.New("proxy name is required")
			}

			resp, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
				Name:        proxyName,
				DisplayName: displayName,
				Icon:        proxyIcon,
			})
			if err != nil {
				return xerrors.Errorf("create workspace proxy: %w", err)
			}

			output, err := formatter.Format(ctx, resp)
			if err != nil {
				return err
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
			Flag:        "no-prompt",
			Description: "Disable all input prompting, and fail if any required flags are missing.",
			Value:       clibase.BoolOf(&noPrompts),
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

// updateProxyResponseFormatter is used for both create and regenerate proxy commands.
type updateProxyResponseFormatter struct {
	onlyToken bool
	formatter *cliui.OutputFormatter
}

func (f *updateProxyResponseFormatter) Format(ctx context.Context, data codersdk.UpdateWorkspaceProxyResponse) (string, error) {
	if f.onlyToken {
		return data.ProxyToken, nil
	}
	return f.formatter.Format(ctx, data)
}

func (f *updateProxyResponseFormatter) AttachOptions(opts *clibase.OptionSet) {
	opts.Add(
		clibase.Option{
			Flag:        "only-token",
			Description: "Only print the token. This is useful for scripting.",
			Value:       clibase.BoolOf(&f.onlyToken),
		},
	)
	f.formatter.AttachOptions(opts)
}

func newUpdateProxyResponseFormatter() *updateProxyResponseFormatter {
	up := &updateProxyResponseFormatter{
		onlyToken: false,
		formatter: cliui.NewOutputFormatter(
			// Text formatter should be human readable.
			cliui.ChangeFormatterData(cliui.TextFormat(), func(data any) (any, error) {
				response, ok := data.(codersdk.UpdateWorkspaceProxyResponse)
				if !ok {
					return nil, xerrors.Errorf("unexpected type %T", data)
				}

				return fmt.Sprintf("Workspace Proxy %q updated successfully.\n"+
					cliui.DefaultStyles.Placeholder.Render("—————————————————————————————————————————————————")+"\n"+
					"Save this authentication token, it will not be shown again.\n"+
					"Token: %s\n", response.Proxy.Name, response.ProxyToken), nil
			}),
			cliui.JSONFormat(),
			// Table formatter expects a slice, make a slice of one.
			cliui.ChangeFormatterData(cliui.TableFormat([]codersdk.UpdateWorkspaceProxyResponse{}, []string{"proxy name", "proxy url", "proxy token"}),
				func(data any) (any, error) {
					response, ok := data.(codersdk.UpdateWorkspaceProxyResponse)
					if !ok {
						return nil, xerrors.Errorf("unexpected type %T", data)
					}
					return []codersdk.UpdateWorkspaceProxyResponse{response}, nil
				}),
		),
	}

	return up
}
