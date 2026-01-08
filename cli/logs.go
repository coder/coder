package cli

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) logs() *serpent.Command {
	var (
		buildNumberArg int64
		followArg      bool
	)
	cmd := &serpent.Command{
		Use:   "logs <workspace>",
		Short: "View logs for a workspace",
		Long:  "View logs for a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Name:          "Build Number",
				Flag:          "build-number",
				FlagShorthand: "n",
				Description:   "Only show logs for a specific build number. Defaults to 0, which maps to the most recent build (build numbers start at 1). Negative values are treated as offsets—for example, -1 refers to the previous build.",
				Value:         serpent.Int64Of(&buildNumberArg),
				Default:       "0",
			},
			{
				Name:          "Follow",
				Flag:          "follow",
				FlagShorthand: "f",
				Description:   "Follow logs as they are emitted.",
				Value:         serpent.BoolOf(&followArg),
				Default:       "false",
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			ws, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("failed to get workspace: %w", err)
			}
			bld := ws.LatestBuild
			buildNumber := buildNumberArg

			// User supplied a negative build number, treat it as an offset from the latest build
			if buildNumber < 0 {
				buildNumber = int64(ws.LatestBuild.BuildNumber) + buildNumberArg
				if buildNumber < 1 {
					return xerrors.Errorf("invalid build number offset: %d latest build number: %d", buildNumberArg, ws.LatestBuild.BuildNumber)
				}
			}

			// Fetch specific build if requested
			if buildNumber > 0 {
				wb, err := client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(ctx, ws.OwnerName, ws.Name, strconv.FormatInt(buildNumber, 10))
				if err != nil {
					return xerrors.Errorf("failed to get build %d: %w", buildNumberArg, err)
				}
				bld = wb
			}
			cliui.Infof(inv.Stdout, "--- Logs for workspace build #%d (ID: %s Template Version: %s) ---", bld.BuildNumber, bld.ID, bld.TemplateVersionName)
			logs, logsCh, err := workspaceLogs(ctx, client, bld, followArg)
			if err != nil {
				return err
			}
			for _, log := range logs {
				_, _ = fmt.Fprintln(inv.Stdout, log.String())
			}
			if followArg {
				_, _ = fmt.Fprintln(inv.Stdout, "--- Streaming logs ---")
				for log := range logsCh {
					_, _ = fmt.Fprintln(inv.Stdout, log.String())
				}
			}
			return nil
		},
	}
	return cmd
}

type logLine struct {
	ts      time.Time
	Content string
}

func (l *logLine) String() string {
	var sb strings.Builder
	_, _ = sb.WriteString(l.ts.Format(time.RFC3339))
	_, _ = sb.WriteString(l.Content)
	return sb.String()
}

// workspaceLogs fetches logs for the given workspace build. If follow is true,
// the returned channel will stream new logs as they are emitted. Otherwise,
// the channel will be closed immediately.
// nolint: revive // control flag is appropriate here
func workspaceLogs(ctx context.Context, client *codersdk.Client, wb codersdk.WorkspaceBuild, follow bool) ([]logLine, <-chan logLine, error) {
	logs := make([]logLine, 0)
	logsCh := make(chan logLine)
	followCh := make(chan logLine)

	var fetchGroup, followGroup errgroup.Group

	buildLogsAfterCh := make(chan int64)
	fetchGroup.Go(func() error {
		var afterID int64
		defer func() {
			if !follow {
				return
			}
			buildLogsAfterCh <- afterID
		}()
		buildLogsC, closer, err := client.WorkspaceBuildLogsAfter(ctx, wb.ID, 0)
		if err != nil {
			return xerrors.Errorf("failed to get build logs: %w", err)
		}
		defer closer.Close()
		for log := range buildLogsC {
			afterID = log.ID
			logsCh <- logLine{
				ts:      log.CreatedAt,
				Content: buildLogToString(log),
			}
		}
		return nil
	})

	if follow {
		followGroup.Go(func() error {
			afterID := <-buildLogsAfterCh
			buildLogsC, closer, err := client.WorkspaceBuildLogsAfter(ctx, wb.ID, afterID)
			if err != nil {
				return xerrors.Errorf("failed to follow build logs: %w", err)
			}
			defer closer.Close()
			for log := range buildLogsC {
				followCh <- logLine{
					ts:      log.CreatedAt,
					Content: buildLogToString(log),
				}
			}
			return nil
		})
	}

	for _, res := range wb.Resources {
		for _, agt := range res.Agents {
			logSrcNames := make(map[uuid.UUID]string)
			for _, src := range agt.LogSources {
				logSrcNames[src.ID] = src.DisplayName
			}
			agentLogsAfterCh := make(chan int64)
			var afterID int64
			fetchGroup.Go(func() error {
				defer func() {
					if !follow {
						return
					}
					agentLogsAfterCh <- afterID
				}()
				agentLogsCh, closer, err := client.WorkspaceAgentLogsAfter(ctx, agt.ID, 0, false)
				if err != nil {
					return xerrors.Errorf("failed to get agent logs: %w", err)
				}
				defer closer.Close()
				for logChunk := range agentLogsCh {
					for _, log := range logChunk {
						afterID = log.ID
						logsCh <- logLine{
							ts:      log.CreatedAt,
							Content: workspaceAgentLogToString(log, agt.Name, logSrcNames[log.SourceID]),
						}
					}
				}
				return nil
			})

			if follow {
				followGroup.Go(func() error {
					afterID := <-agentLogsAfterCh
					agentLogsCh, closer, err := client.WorkspaceAgentLogsAfter(ctx, agt.ID, afterID, true)
					if err != nil {
						return xerrors.Errorf("failed to follow agent logs: %w", err)
					}
					defer closer.Close()
					for logChunk := range agentLogsCh {
						for _, log := range logChunk {
							followCh <- logLine{
								ts:      log.CreatedAt,
								Content: workspaceAgentLogToString(log, agt.Name, logSrcNames[log.SourceID]),
							}
						}
					}
					return nil
				})
			}
		}
	}

	logsDone := make(chan struct{})
	go func() {
		defer close(logsDone)
		for log := range logsCh {
			logs = append(logs, log)
		}
	}()

	err := fetchGroup.Wait()
	close(logsCh)
	<-logsDone

	slices.SortFunc(logs, func(a, b logLine) int {
		return a.ts.Compare(b.ts)
	})

	if follow {
		go func() {
			_ = followGroup.Wait()
			close(followCh)
		}()
	} else {
		close(followCh)
	}

	return logs, followCh, err
}

func buildLogToString(log codersdk.ProvisionerJobLog) string {
	var sb strings.Builder
	_, _ = sb.WriteString(" [")
	_, _ = sb.WriteString(string(log.Level))
	_, _ = sb.WriteString("] [")
	_, _ = sb.WriteString("provisioner|")
	_, _ = sb.WriteString(log.Stage)
	_, _ = sb.WriteString("] ")
	_, _ = sb.WriteString(log.Output)
	return sb.String()
}

func workspaceAgentLogToString(log codersdk.WorkspaceAgentLog, agtName, srcName string) string {
	var sb strings.Builder
	_, _ = sb.WriteString(" [")
	_, _ = sb.WriteString(string(log.Level))
	_, _ = sb.WriteString("] [")
	_, _ = sb.WriteString("agent.")
	_, _ = sb.WriteString(agtName)
	_, _ = sb.WriteString("|")
	_, _ = sb.WriteString(srcName)
	_, _ = sb.WriteString("] ")
	_, _ = sb.WriteString(log.Output)
	return sb.String()
}
