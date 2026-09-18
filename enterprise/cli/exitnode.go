package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/pretty"
	"github.com/coder/serpent"

	"github.com/coder/coder/v2/buildinfo"
	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clilog"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/exitnode"
	"github.com/coder/coder/v2/enterprise/exitnode/exitnodesdk"
)

func (r *RootCmd) exitNode() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "exit-node",
		Short: "Exit nodes terminate workspace egress, enforce policy, and report flows.",
		Long: "Exit nodes are tailnet peers that workspace agents route outbound TCP " +
			"traffic through. Each flow is checked against a policy and reported to coderd.",
		Aliases: []string{"exitnode"},
		Hidden:  true,
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.exitNodeServer(),
			r.exitNodeCreate(),
			r.exitNodeList(),
			r.exitNodeDelete(),
		},
	}
	return cmd
}

func (r *RootCmd) exitNodeCreate() *serpent.Command {
	var (
		orgContext  = agpl.NewOrganizationContext()
		displayName string
		onlyToken   bool
	)
	cmd := &serpent.Command{
		Use:   "create <name>",
		Short: "Create an exit node and print its token",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}

			resp, err := client.CreateExitNode(ctx, org.ID, codersdk.CreateExitNodeRequest{
				Name:        inv.Args[0],
				DisplayName: displayName,
			})
			if err != nil {
				return xerrors.Errorf("create exit node: %w", err)
			}

			if onlyToken {
				_, err = fmt.Fprintln(inv.Stdout, resp.Token)
				return err
			}

			_, err = fmt.Fprintf(inv.Stdout,
				"Exit node %[1]q created successfully.\n"+
					pretty.Sprint(cliui.DefaultStyles.Placeholder, strings.Repeat("-", 49))+"\n"+
					"Save this authentication token, it will not be shown again.\n"+
					"Token: %[2]s\n"+
					"\n"+
					"Tailnet address: %[3]s\n"+
					"\n"+
					"Start the exit node by running:\n"+
					cliui.Code("CODER_EXIT_NODE_TOKEN=%[2]s coder exit-node server --primary-access-url %[4]s --policy policy.yaml")+
					pretty.Sprint(cliui.DefaultStyles.Placeholder, "")+"\n",
				resp.Name, resp.Token, resp.TailnetAddress, client.URL.String(),
			)
			return err
		},
	}
	cmd.Options.Add(
		serpent.Option{
			Flag:        "display-name",
			Description: "Display name of the exit node. Defaults to the name.",
			Value:       serpent.StringOf(&displayName),
		},
		serpent.Option{
			Flag:        "only-token",
			Description: "Only print the token. This is useful for scripting.",
			Value:       serpent.BoolOf(&onlyToken),
		},
	)
	orgContext.AttachOptions(cmd)
	return cmd
}

func (r *RootCmd) exitNodeList() *serpent.Command {
	var (
		orgContext = agpl.NewOrganizationContext()
		formatter  = cliui.NewOutputFormatter(
			cliui.TableFormat([]codersdk.ExitNode{}, []string{"name", "display name", "tailnet address", "version", "last seen at"}),
			cliui.JSONFormat(),
		)
	)
	cmd := &serpent.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List exit nodes in an organization",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}
			nodes, err := client.ExitNodes(ctx, org.ID)
			if err != nil {
				return xerrors.Errorf("list exit nodes: %w", err)
			}
			if len(nodes) == 0 {
				cliui.Infof(inv.Stderr, "No exit nodes found.")
				return nil
			}
			output, err := formatter.Format(ctx, nodes)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, output)
			return err
		},
	}
	formatter.AttachOptions(&cmd.Options)
	orgContext.AttachOptions(cmd)
	return cmd
}

func (r *RootCmd) exitNodeDelete() *serpent.Command {
	orgContext := agpl.NewOrganizationContext()
	cmd := &serpent.Command{
		Use:   "delete <name|id>",
		Short: "Delete an exit node",
		Options: serpent.OptionSet{
			cliui.SkipPromptOption(),
		},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}
			node, err := client.ExitNodeByName(ctx, org.ID, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("fetch exit node %q: %w", inv.Args[0], err)
			}
			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Delete this exit node: %s?", pretty.Sprint(cliui.DefaultStyles.Code, node.Name)),
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}
			if err := client.DeleteExitNode(ctx, org.ID, node.Name); err != nil {
				return xerrors.Errorf("delete exit node %q: %w", node.Name, err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Exit node %q deleted successfully\n", node.Name)
			return nil
		},
	}
	orgContext.AttachOptions(cmd)
	return cmd
}

func (r *RootCmd) exitNodeServer() *serpent.Command {
	var (
		token               string
		primaryAccessURL    serpent.URL
		policyPath          string
		listenPort          int64
		prometheusAddress   string
		wireguardEndpoints  []string
		wireguardListenPort int64
		blockDirect         bool
		verbose             bool
	)
	cmd := &serpent.Command{
		Use:   "server",
		Short: "Run an exit node",
		Long: "Run an exit node. The node registers with coderd using its token, joins the " +
			"tailnet at a deterministic address derived from its ID, and accepts HTTP CONNECT " +
			"requests from workspace agents on the listen port. Send SIGHUP to reload the policy file.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			exitNodeID, err := exitNodeIDFromToken(token)
			if err != nil {
				return err
			}

			logOpts := []clilog.Option{clilog.WithHuman("/dev/stderr")}
			if verbose {
				logOpts = append(logOpts, clilog.WithVerbose())
			}
			logger, closeLogger, err := clilog.New(logOpts...).Build(inv)
			if err != nil {
				logger = slog.Make(sloghuman.Sink(inv.Stderr))
				logger.Error(ctx, "failed to initialize logger", slog.Error(err))
			} else {
				defer closeLogger()
			}

			policy, err := exitnode.LoadPolicyFile(policyPath)
			if err != nil {
				return xerrors.Errorf("load policy: %w", err)
			}

			notifyCtx, notifyStop := inv.SignalNotifyContext(ctx, exitNodeStopSignals...)
			defer notifyStop()

			var prometheusRegistry *prometheus.Registry
			if prometheusAddress != "" {
				prometheusRegistry = prometheus.NewRegistry()
				prometheusRegistry.MustRegister(collectors.NewGoCollector())
				prometheusRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
				closeFunc := agpl.ServeHandler(ctx, logger, promhttp.InstrumentMetricHandler(
					prometheusRegistry, promhttp.HandlerFor(prometheusRegistry, promhttp.HandlerOpts{}),
				), prometheusAddress, "prometheus")
				defer closeFunc()
			}

			client := exitnodesdk.New(primaryAccessURL.Value(), token)
			client.SDKClient.SetLogger(logger.Named("sdk"))
			headerTransport, err := r.HeaderTransport(ctx, primaryAccessURL.Value())
			if err != nil {
				return xerrors.Errorf("configure header transport: %w", err)
			}
			headerTransport.Transport = http.DefaultTransport
			client.SDKClient.HTTPClient.Transport = headerTransport

			cliui.Infof(inv.Stdout, "Starting exit node %s (tailnet address %s)",
				exitNodeID, exitnode.TailnetAddrForID(exitNodeID))

			srv, err := exitnode.New(ctx, logger, exitnode.Options{
				Client:                       client,
				ExitNodeID:                   exitNodeID,
				Policy:                       policy,
				ListenPort:                   int(listenPort),
				PrometheusRegistry:           prometheusRegistry,
				Version:                      buildinfo.Version(),
				Hostname:                     cliutil.Hostname(),
				WireguardEndpointsAdvertised: wireguardEndpoints,
				WireguardListenPort:          uint16(wireguardListenPort), //nolint:gosec // validated below
				BlockEndpoints:               blockDirect,
			})
			if err != nil {
				return xerrors.Errorf("start exit node: %w", err)
			}
			defer func() {
				if err := srv.Close(); err != nil {
					logger.Warn(context.Background(), "failed to close exit node cleanly", slog.Error(err))
				}
			}()

			reloadCh := make(chan os.Signal, 1)
			if len(exitNodeReloadSignals) > 0 {
				signal.Notify(reloadCh, exitNodeReloadSignals...)
				defer signal.Stop(reloadCh)
			}

			cliui.Infof(inv.Stdout, "\n==> Logs will stream in below (press ctrl+c to gracefully exit):")

			waitErr := make(chan error, 1)
			go func() { waitErr <- srv.Wait() }()

			for {
				select {
				case <-reloadCh:
					if err := srv.ReloadPolicy(); err != nil {
						logger.Error(ctx, "policy reload failed; previous policy remains active", slog.Error(err))
					}
				case err := <-waitErr:
					if err != nil {
						return xerrors.Errorf("exit node stopped: %w", err)
					}
					return nil
				case <-notifyCtx.Done():
					_, _ = fmt.Fprintln(inv.Stdout, cliui.Bold("Interrupt caught, gracefully exiting."))
					return nil
				}
			}
		},
	}
	cmd.Options.Add(
		serpent.Option{
			Name:        "Exit Node Token",
			Flag:        "token",
			Env:         "CODER_EXIT_NODE_TOKEN",
			Description: "Authentication token for the exit node, as printed by 'coder exit-node create'.",
			Required:    true,
			Value:       serpent.StringOf(&token),
		},
		serpent.Option{
			Name:        "Coderd (Primary) Access URL",
			Flag:        "primary-access-url",
			Env:         "CODER_PRIMARY_ACCESS_URL",
			Description: "URL to communicate with coderd. This should match the access URL of the Coder deployment.",
			Required:    true,
			Value: serpent.Validate(&primaryAccessURL, func(value *serpent.URL) error {
				if value.Scheme != "http" && value.Scheme != "https" {
					return xerrors.Errorf("'--primary-access-url' value must be http or https: url=%s", value.String())
				}
				return nil
			}),
		},
		serpent.Option{
			Flag:        "policy",
			Env:         "CODER_EXIT_NODE_POLICY",
			Description: "Path to the YAML policy file. Reloaded on SIGHUP.",
			Required:    true,
			Value:       serpent.StringOf(&policyPath),
		},
		serpent.Option{
			Flag:        "listen-port",
			Env:         "CODER_EXIT_NODE_LISTEN_PORT",
			Description: "Tailnet port to accept CONNECT requests on.",
			Default:     fmt.Sprint(codersdk.ExitNodeTailnetPort),
			Value:       serpent.Int64Of(&listenPort),
		},
		serpent.Option{
			Flag:        "prometheus-address",
			Env:         "CODER_EXIT_NODE_PROMETHEUS_ADDRESS",
			Description: "Address to serve Prometheus metrics on. Disabled when empty.",
			Value:       serpent.StringOf(&prometheusAddress),
		},
		serpent.Option{
			Flag: "wireguard-endpoint",
			Env:  "CODER_EXIT_NODE_WIREGUARD_ENDPOINTS",
			Description: "Public ip:port of this node's WireGuard listener, advertised to agents so they " +
				"exempt it from egress enforcement. May be repeated.",
			Value: serpent.StringArrayOf(&wireguardEndpoints),
		},
		serpent.Option{
			Flag:        "wireguard-listen-port",
			Env:         "CODER_EXIT_NODE_WIREGUARD_LISTEN_PORT",
			Description: "Fixed local UDP port for WireGuard. Use with --wireguard-endpoint. 0 picks a random port.",
			Default:     "0",
			Value: serpent.Validate(serpent.Int64Of(&wireguardListenPort), func(v *serpent.Int64) error {
				if v.Value() < 0 || v.Value() > 65535 {
					return xerrors.Errorf("port %d out of range", v.Value())
				}
				return nil
			}),
		},
		serpent.Option{
			Flag:        "block-direct-connections",
			Env:         "CODER_EXIT_NODE_BLOCK_DIRECT",
			Description: "Force all agent traffic through DERP relays.",
			Value:       serpent.BoolOf(&blockDirect),
		},
		serpent.Option{
			Flag:        "verbose",
			Env:         "CODER_EXIT_NODE_VERBOSE",
			Description: "Output debug-level logs.",
			Value:       serpent.BoolOf(&verbose),
		},
	)
	return cmd
}

// exitNodeIDFromToken extracts the exit node ID from an "<id>:<secret>"
// token without exposing the secret in the error.
func exitNodeIDFromToken(token string) (uuid.UUID, error) {
	idStr, _, ok := strings.Cut(token, ":")
	if !ok {
		return uuid.Nil, xerrors.New("token must have the form <exit node ID>:<secret>")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("token does not start with a valid exit node ID: %w", err)
	}
	return id, nil
}
