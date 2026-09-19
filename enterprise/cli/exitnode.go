package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"

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
	"github.com/coder/coder/v2/enterprise/exitnode/yamlpolicy"
)

// exitNodeStopSignals shut the exit node down. SIGHUP is deliberately absent
// because it reloads the policy instead. Windows never delivers SIGHUP, so
// there the policy can only be changed by restarting the process.
var exitNodeStopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

func (r *RootCmd) exitNode() *serpent.Command {
	return &serpent.Command{
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
			r.exitNodeReplicas(),
			r.exitNodeDelete(),
		},
	}
}

// exitNodeOrgClient initializes the client and resolves the selected
// organization for the organization-scoped exit node commands.
func (r *RootCmd) exitNodeOrgClient(inv *serpent.Invocation, orgContext *agpl.OrganizationContext) (*codersdk.Client, codersdk.Organization, error) {
	client, err := r.InitClient(inv)
	if err != nil {
		return nil, codersdk.Organization{}, err
	}
	org, err := orgContext.Selected(inv, client)
	if err != nil {
		return nil, codersdk.Organization{}, xerrors.Errorf("current organization: %w", err)
	}
	return client, org, nil
}

func (r *RootCmd) exitNodeCreate() *serpent.Command {
	var (
		orgContext  = agpl.NewOrganizationContext()
		displayName string
		onlyToken   bool
	)
	cmd := &serpent.Command{
		Use:        "create <name>",
		Short:      "Create an exit node and print its token",
		Middleware: serpent.RequireNArgs(1),
		Options: serpent.OptionSet{
			{Flag: "display-name", Description: "Display name of the exit node. Defaults to the name.", Value: serpent.StringOf(&displayName)},
			{Flag: "only-token", Description: "Only print the token. This is useful for scripting.", Value: serpent.BoolOf(&onlyToken)},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, org, err := r.exitNodeOrgClient(inv, orgContext)
			if err != nil {
				return err
			}
			resp, err := client.CreateExitNode(inv.Context(), org.ID, codersdk.CreateExitNodeRequest{
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
					"Start the exit node by running:\n"+
					cliui.Code("CODER_EXIT_NODE_TOKEN=%[2]s coder exit-node server --primary-access-url %[3]s --policy policy.yaml")+
					pretty.Sprint(cliui.DefaultStyles.Placeholder, "")+"\n",
				resp.Name, resp.Token, client.URL.String(),
			)
			return err
		},
	}
	orgContext.AttachOptions(cmd)
	return cmd
}

type exitNodeListRow struct {
	codersdk.ExitNode `table:"exit_node,recursive_inline"`
	ReplicaCount      int `json:"-" table:"replicas"`
}

func exitNodeListRows(nodes []codersdk.ExitNode) []exitNodeListRow {
	rows := make([]exitNodeListRow, len(nodes))
	for i, node := range nodes {
		rows[i] = exitNodeListRow{ExitNode: node, ReplicaCount: len(node.Replicas)}
	}
	return rows
}

func (r *RootCmd) exitNodeList() *serpent.Command {
	var (
		orgContext = agpl.NewOrganizationContext()
		formatter  = cliui.NewOutputFormatter(
			cliui.TableFormat([]exitNodeListRow{}, []string{"name", "display name", "status", "replicas", "policy mismatch"}),
			cliui.JSONFormat(),
		)
	)
	cmd := &serpent.Command{
		Use:        "list",
		Aliases:    []string{"ls"},
		Short:      "List exit nodes in an organization",
		Middleware: serpent.RequireNArgs(0),
		Handler: func(inv *serpent.Invocation) error {
			client, org, err := r.exitNodeOrgClient(inv, orgContext)
			if err != nil {
				return err
			}
			nodes, err := client.ExitNodes(inv.Context(), org.ID)
			if err != nil {
				return xerrors.Errorf("list exit nodes: %w", err)
			}
			if len(nodes) == 0 {
				cliui.Infof(inv.Stderr, "No exit nodes found.")
				return nil
			}
			output, err := formatter.Format(inv.Context(), exitNodeListRows(nodes))
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

type exitNodeReplicaRow struct {
	Hostname       string                         `table:"hostname,default_sort"`
	Status         codersdk.ExitNodeReplicaStatus `table:"status"`
	Version        string                         `table:"version"`
	TailnetAddress string                         `table:"tailnet address"`
	PolicyHash     string                         `table:"policy hash"`
	UpdatedAt      string                         `table:"updated at"`
}

func exitNodeReplicaRows(replicas []codersdk.ExitNodeReplica) []exitNodeReplicaRow {
	rows := make([]exitNodeReplicaRow, len(replicas))
	for i, replica := range replicas {
		hash := replica.PolicyHash
		if len(hash) > 12 {
			hash = hash[:12]
		}
		rows[i] = exitNodeReplicaRow{
			Hostname:       replica.Hostname,
			Status:         replica.Status,
			Version:        replica.Version,
			TailnetAddress: replica.TailnetAddress,
			PolicyHash:     hash,
			UpdatedAt:      replica.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}
	return rows
}

func (r *RootCmd) exitNodeReplicas() *serpent.Command {
	var (
		orgContext = agpl.NewOrganizationContext()
		formatter  = cliui.NewOutputFormatter(
			cliui.TableFormat([]exitNodeReplicaRow{}, []string{"hostname", "status", "version", "tailnet address", "policy hash", "updated at"}),
			cliui.JSONFormat(),
		)
	)
	cmd := &serpent.Command{
		Use:        "replicas <name|id>",
		Short:      "List replicas of an exit node",
		Middleware: serpent.RequireNArgs(1),
		Handler: func(inv *serpent.Invocation) error {
			client, org, err := r.exitNodeOrgClient(inv, orgContext)
			if err != nil {
				return err
			}
			node, err := client.ExitNodeByName(inv.Context(), org.ID, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("fetch exit node %q: %w", inv.Args[0], err)
			}
			if len(node.Replicas) == 0 {
				cliui.Infof(inv.Stderr, "No replicas found for exit node %q.", node.Name)
				return nil
			}
			data := any(exitNodeReplicaRows(node.Replicas))
			if formatter.FormatID() == "json" {
				data = node.Replicas
			}
			output, err := formatter.Format(inv.Context(), data)
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
		Use:        "delete <name|id>",
		Short:      "Delete an exit node",
		Options:    serpent.OptionSet{cliui.SkipPromptOption()},
		Middleware: serpent.RequireNArgs(1),
		Handler: func(inv *serpent.Invocation) error {
			client, org, err := r.exitNodeOrgClient(inv, orgContext)
			if err != nil {
				return err
			}
			node, err := client.ExitNodeByName(inv.Context(), org.ID, inv.Args[0])
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
			if err := client.DeleteExitNode(inv.Context(), org.ID, node.Name); err != nil {
				return xerrors.Errorf("delete exit node %q: %w", node.Name, err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Exit node %q deleted successfully\n", node.Name)
			return nil
		},
	}
	orgContext.AttachOptions(cmd)
	return cmd
}

func parseUpstreamDNSServers(values []string) ([]netip.AddrPort, error) {
	servers := make([]netip.AddrPort, 0, len(values))
	for _, value := range values {
		if addr, err := netip.ParseAddr(value); err == nil {
			servers = append(servers, netip.AddrPortFrom(addr.Unmap(), 53))
			continue
		}
		server, err := netip.ParseAddrPort(value)
		if err != nil || server.Port() == 0 {
			return nil, xerrors.Errorf("parse --upstream-dns %q as ip[:port]", value)
		}
		servers = append(servers, netip.AddrPortFrom(server.Addr().Unmap(), server.Port()))
	}
	return servers, nil
}

func newUpstreamDNSResolver(servers []netip.AddrPort) *net.Resolver {
	var next atomic.Uint64
	dialer := &net.Dialer{}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			server := servers[(next.Add(1)-1)%uint64(len(servers))]
			return dialer.DialContext(ctx, network, server.String())
		},
	}
}

func (r *RootCmd) exitNodeServer() *serpent.Command {
	var (
		token                string
		primaryAccessURL     serpent.URL
		policyPath           string
		listenPort           int64
		prometheusAddress    string
		wireguardEndpoints   []string
		wireguardListenPort  int64
		blockDirect          bool
		provisionalHostAllow bool
		upstreamDNS          []string
		verbose              bool
		replicaIDOverride    string
	)
	return &serpent.Command{
		Use:   "server",
		Short: "Run an exit node",
		Long: "Run an exit node replica. The replica registers with coderd using its token, joins the " +
			"tailnet at a deterministic address derived from its replica ID, and accepts HTTP CONNECT " +
			"requests from workspace agents on the listen port. Send SIGHUP to reload the policy file.",
		Middleware: serpent.RequireNArgs(0),
		Options: serpent.OptionSet{
			{
				Name:        "Exit Node Token",
				Flag:        "token",
				Env:         "CODER_EXIT_NODE_TOKEN",
				Description: "Authentication token for the exit node, as printed by 'coder exit-node create'.",
				Required:    true,
				Value:       serpent.StringOf(&token),
			},
			{
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
			{
				Flag:        "policy",
				Env:         "CODER_EXIT_NODE_POLICY",
				Description: "Path to the YAML policy file. Reloaded on SIGHUP.",
				Required:    true,
				Value:       serpent.StringOf(&policyPath),
			},
			{
				Flag:        "replica-id",
				Description: "Replica ID override. By default a fresh ID is generated at startup.",
				Value:       serpent.StringOf(&replicaIDOverride),
			},
			{
				Flag:        "listen-port",
				Env:         "CODER_EXIT_NODE_LISTEN_PORT",
				Description: "Tailnet port to accept CONNECT requests on.",
				Default:     fmt.Sprint(codersdk.ExitNodeTailnetPort),
				Value:       serpent.Int64Of(&listenPort),
			},
			{
				Flag:        "prometheus-address",
				Env:         "CODER_EXIT_NODE_PROMETHEUS_ADDRESS",
				Description: "Address to serve Prometheus metrics on. Disabled when empty.",
				Value:       serpent.StringOf(&prometheusAddress),
			},
			{
				Flag: "wireguard-endpoint",
				Env:  "CODER_EXIT_NODE_WIREGUARD_ENDPOINTS",
				Description: "Public ip:port of this node's WireGuard listener, advertised to agents so they " +
					"exempt it from egress enforcement. May be repeated.",
				Value: serpent.StringArrayOf(&wireguardEndpoints),
			},
			{
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
			{
				Flag:        "block-direct-connections",
				Env:         "CODER_EXIT_NODE_BLOCK_DIRECT",
				Description: "Force all agent traffic through DERP relays.",
				Value:       serpent.BoolOf(&blockDirect),
			},
			{
				Flag:        "provisional-host-allow",
				Env:         "CODER_EXIT_NODE_PROVISIONAL_HOST_ALLOW",
				Description: "Allow host-based allow rules to provisionally match IP-literal targets before sniffing. This weakens host enforcement and may permit traffic before the host is verified.",
				Value:       serpent.BoolOf(&provisionalHostAllow),
			},
			{
				Flag:        "upstream-dns",
				Env:         "CODER_EXIT_NODE_UPSTREAM_DNS",
				Description: "Upstream DNS server as ip[:port]. May be repeated. Configures both CONNECT hostname resolution and DNS streams. By default, the exit node's own resolver view decides what a name means.",
				Value:       serpent.StringArrayOf(&upstreamDNS),
			},
			{
				Flag:        "verbose",
				Env:         "CODER_EXIT_NODE_VERBOSE",
				Description: "Output debug-level logs.",
				Value:       serpent.BoolOf(&verbose),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			// Only the ID is needed locally; the secret stays in the token
			// and must not surface in errors.
			idStr, _, ok := strings.Cut(token, ":")
			if !ok {
				return xerrors.New("token must have the form <exit node ID>:<secret>")
			}
			exitNodeID, err := uuid.Parse(idStr)
			if err != nil {
				return xerrors.Errorf("token does not start with a valid exit node ID: %w", err)
			}

			replicaID := uuid.New()
			if replicaIDOverride != "" {
				replicaID, err = uuid.Parse(replicaIDOverride)
				if err != nil {
					return xerrors.Errorf("parse --replica-id: %w", err)
				}
				if replicaID == uuid.Nil {
					return xerrors.New("--replica-id must not be nil")
				}
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

			policy, err := yamlpolicy.Load(policyPath)
			if err != nil {
				return xerrors.Errorf("load policy: %w", err)
			}

			var (
				resolver     exitnode.HostResolver
				dnsExchanger exitnode.DNSExchanger
			)
			if len(upstreamDNS) > 0 {
				servers, err := parseUpstreamDNSServers(upstreamDNS)
				if err != nil {
					return err
				}
				resolver = newUpstreamDNSResolver(servers)
				dnsExchanger = &exitnode.StaticDNSExchanger{Servers: servers}
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

			cliui.Infof(inv.Stdout, "Starting exit node replica %s (tailnet address %s)",
				replicaID, exitnode.TailnetAddrForID(replicaID))

			srv, err := exitnode.New(ctx, logger, exitnode.Options{
				Client:                       client,
				ExitNodeID:                   exitNodeID,
				ReplicaID:                    replicaID,
				Policy:                       policy,
				ListenPort:                   int(listenPort),
				PrometheusRegistry:           prometheusRegistry,
				Version:                      buildinfo.Version(),
				Hostname:                     cliutil.Hostname(),
				WireguardEndpointsAdvertised: wireguardEndpoints,
				WireguardListenPort:          uint16(wireguardListenPort), //nolint:gosec // Validated by the option.
				BlockEndpoints:               blockDirect,
				ProvisionalHostAllow:         provisionalHostAllow,
				Resolver:                     resolver,
				DNSExchanger:                 dnsExchanger,
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
			signal.Notify(reloadCh, syscall.SIGHUP)
			defer signal.Stop(reloadCh)

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
}
