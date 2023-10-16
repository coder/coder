package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"
	"tailscale.com/types/netlogtype"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/codersdk"
)

// vscodeSSH is used by the Coder VS Code extension to establish
// a connection to a workspace.
//
// This command needs to remain stable for compatibility with
// various VS Code versions, so it's kept separate from our
// standard SSH command.
func (r *RootCmd) vscodeSSH() *clibase.Cmd {
	var (
		sessionTokenFile    string
		urlFile             string
		networkInfoDir      string
		networkInfoInterval time.Duration
	)
	cmd := &clibase.Cmd{
		// A SSH config entry is added by the VS Code extension that
		// passes %h to ProxyCommand. The prefix of `coder-vscode--`
		// is a magical string represented in our VS Code extension.
		// It's not important here, only the delimiter `--` is.
		Use:        "vscodessh <coder-vscode--<owner>--<workspace>--<agent?>>",
		Hidden:     true,
		Middleware: clibase.RequireNArgs(1),
		Handler: func(inv *clibase.Invocation) error {
			if networkInfoDir == "" {
				return xerrors.New("network-info-dir must be specified")
			}
			if sessionTokenFile == "" {
				return xerrors.New("session-token-file must be specified")
			}
			if urlFile == "" {
				return xerrors.New("url-file must be specified")
			}

			fs, ok := inv.Context().Value("fs").(afero.Fs)
			if !ok {
				fs = afero.NewOsFs()
			}

			sessionToken, err := afero.ReadFile(fs, sessionTokenFile)
			if err != nil {
				return xerrors.Errorf("read session token: %w", err)
			}
			rawURL, err := afero.ReadFile(fs, urlFile)
			if err != nil {
				return xerrors.Errorf("read url: %w", err)
			}
			serverURL, err := url.Parse(string(rawURL))
			if err != nil {
				return xerrors.Errorf("parse url: %w", err)
			}

			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			err = fs.MkdirAll(networkInfoDir, 0o700)
			if err != nil {
				return xerrors.Errorf("mkdir: %w", err)
			}

			client := codersdk.New(serverURL)
			client.SetSessionToken(string(sessionToken))

			// This adds custom headers to the request!
			err = r.setClient(ctx, client, serverURL)
			if err != nil {
				return xerrors.Errorf("set client: %w", err)
			}

			parts := strings.Split(inv.Args[0], "--")
			if len(parts) < 3 {
				return xerrors.Errorf("invalid argument format. must be: coder-vscode--<owner>--<name>--<agent?>")
			}
			owner := parts[1]
			name := parts[2]

			workspace, err := client.WorkspaceByOwnerAndName(ctx, owner, name, codersdk.WorkspaceOptions{})
			if err != nil {
				return xerrors.Errorf("find workspace: %w", err)
			}

			var agent codersdk.WorkspaceAgent
			var found bool
			for _, resource := range workspace.LatestBuild.Resources {
				if len(resource.Agents) == 0 {
					continue
				}
				for _, resourceAgent := range resource.Agents {
					// If an agent name isn't included we default to
					// the first agent!
					if len(parts) != 4 {
						agent = resourceAgent
						found = true
						break
					}
					if resourceAgent.Name != parts[3] {
						continue
					}
					agent = resourceAgent
					found = true
					break
				}
				if found {
					break
				}
			}

			var logger slog.Logger
			if r.verbose {
				logger = slog.Make(sloghuman.Sink(inv.Stdout)).Leveled(slog.LevelDebug)
			}

			if r.disableDirect {
				_, _ = fmt.Fprintln(inv.Stderr, "Direct connections disabled.")
			}
			agentConn, err := client.DialWorkspaceAgent(ctx, agent.ID, &codersdk.DialWorkspaceAgentOptions{
				Logger:         logger,
				BlockEndpoints: r.disableDirect,
			})
			if err != nil {
				return xerrors.Errorf("dial workspace agent: %w", err)
			}
			defer agentConn.Close()

			agentConn.AwaitReachable(ctx)
			rawSSH, err := agentConn.SSH(ctx)
			if err != nil {
				return err
			}
			defer rawSSH.Close()

			// Copy SSH traffic over stdio.
			go func() {
				_, _ = io.Copy(inv.Stdout, rawSSH)
			}()
			go func() {
				_, _ = io.Copy(rawSSH, inv.Stdin)
			}()

			// The VS Code extension obtains the PID of the SSH process to
			// read the file below which contains network information to display.
			//
			// We get the parent PID because it's assumed `ssh` is calling this
			// command via the ProxyCommand SSH option.
			networkInfoFilePath := filepath.Join(networkInfoDir, fmt.Sprintf("%d.json", os.Getppid()))

			statsErrChan := make(chan error, 1)
			cb := func(start, end time.Time, virtual, _ map[netlogtype.Connection]netlogtype.Counts) {
				sendErr := func(err error) {
					select {
					case statsErrChan <- err:
					default:
					}
				}

				stats, err := collectNetworkStats(ctx, agentConn, start, end, virtual)
				if err != nil {
					sendErr(err)
					return
				}

				rawStats, err := json.Marshal(stats)
				if err != nil {
					sendErr(err)
					return
				}
				err = afero.WriteFile(fs, networkInfoFilePath, rawStats, 0o600)
				if err != nil {
					sendErr(err)
					return
				}
			}

			now := time.Now()
			cb(now, now.Add(time.Nanosecond), map[netlogtype.Connection]netlogtype.Counts{}, map[netlogtype.Connection]netlogtype.Counts{})
			agentConn.SetConnStatsCallback(networkInfoInterval, 2048, cb)

			select {
			case <-ctx.Done():
				return nil
			case err := <-statsErrChan:
				return err
			}
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:        "network-info-dir",
			Description: "Specifies a directory to write network information periodically.",
			Value:       clibase.StringOf(&networkInfoDir),
		},
		{
			Flag:        "session-token-file",
			Description: "Specifies a file that contains a session token.",
			Value:       clibase.StringOf(&sessionTokenFile),
		},
		{
			Flag:        "url-file",
			Description: "Specifies a file that contains the Coder URL.",
			Value:       clibase.StringOf(&urlFile),
		},
		{
			Flag:        "network-info-interval",
			Description: "Specifies the interval to update network information.",
			Default:     "5s",
			Value:       clibase.DurationOf(&networkInfoInterval),
		},
	}
	return cmd
}

type sshNetworkStats struct {
	P2P              bool               `json:"p2p"`
	Latency          float64            `json:"latency"`
	PreferredDERP    string             `json:"preferred_derp"`
	DERPLatency      map[string]float64 `json:"derp_latency"`
	UploadBytesSec   int64              `json:"upload_bytes_sec"`
	DownloadBytesSec int64              `json:"download_bytes_sec"`
}

func collectNetworkStats(ctx context.Context, agentConn *codersdk.WorkspaceAgentConn, start, end time.Time, counts map[netlogtype.Connection]netlogtype.Counts) (*sshNetworkStats, error) {
	latency, p2p, pingResult, err := agentConn.Ping(ctx)
	if err != nil {
		return nil, err
	}
	node := agentConn.Node()
	derpMap := agentConn.DERPMap()
	derpLatency := map[string]float64{}

	// Convert DERP region IDs to friendly names for display in the UI.
	for rawRegion, latency := range node.DERPLatency {
		regionParts := strings.SplitN(rawRegion, "-", 2)
		regionID, err := strconv.Atoi(regionParts[0])
		if err != nil {
			continue
		}
		region, found := derpMap.Regions[regionID]
		if !found {
			// It's possible that a workspace agent is using an old DERPMap
			// and reports regions that do not exist. If that's the case,
			// report the region as unknown!
			region = &tailcfg.DERPRegion{
				RegionID:   regionID,
				RegionName: fmt.Sprintf("Unnamed %d", regionID),
			}
		}
		// Convert the microseconds to milliseconds.
		derpLatency[region.RegionName] = latency * 1000
	}

	totalRx := uint64(0)
	totalTx := uint64(0)
	for _, stat := range counts {
		totalRx += stat.RxBytes
		totalTx += stat.TxBytes
	}
	// Tracking the time since last request is required because
	// ExtractTrafficStats() resets its counters after each call.
	dur := end.Sub(start)
	uploadSecs := float64(totalTx) / dur.Seconds()
	downloadSecs := float64(totalRx) / dur.Seconds()

	// Sometimes the preferred DERP doesn't match the one we're actually
	// connected with. Perhaps because the agent prefers a different DERP and
	// we're using that server instead.
	preferredDerpID := node.PreferredDERP
	if pingResult.DERPRegionID != 0 {
		preferredDerpID = pingResult.DERPRegionID
	}
	preferredDerp, ok := derpMap.Regions[preferredDerpID]
	preferredDerpName := fmt.Sprintf("Unnamed %d", preferredDerpID)
	if ok {
		preferredDerpName = preferredDerp.RegionName
	}
	if _, ok := derpLatency[preferredDerpName]; !ok {
		derpLatency[preferredDerpName] = 0
	}

	return &sshNetworkStats{
		P2P:              p2p,
		Latency:          float64(latency.Microseconds()) / 1000,
		PreferredDERP:    preferredDerpName,
		DERPLatency:      derpLatency,
		UploadBytesSec:   int64(uploadSecs),
		DownloadBytesSec: int64(downloadSecs),
	}, nil
}
