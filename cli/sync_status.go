package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/cli/cliui"
)

type outputFormat string

const (
	outputFormatHuman outputFormat = "human"
	outputFormatJSON  outputFormat = "json"
)

func (*RootCmd) syncStatus() *serpent.Command {
	var output string

	cmd := &serpent.Command{
		Use:   "status <unit>",
		Short: "Show unit status and dependency state",
		Long:  "Show the current status of a unit, whether it is ready to start, and lists its dependencies. Shows which dependencies are satisfied and which are still pending. Supports multiple output formats.",
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			if len(i.Args) != 1 {
				return xerrors.New("exactly one unit name is required")
			}
			unit := i.Args[0]

			client, err := agentsocket.NewClient(ctx)
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			statusResp, err := client.SyncStatus(ctx, unit)
			if err != nil {
				return xerrors.Errorf("get status failed: %w", err)
			}

			switch outputFormat(output) {
			case outputFormatJSON:
				return outputJSON(i.Stdout, unit, statusResp)
			default:
				return outputHuman(i.Stdout, unit, statusResp)
			}
		},
	}

	cmd.Options = append(cmd.Options,
		serpent.Option{
			Flag:          "output",
			FlagShorthand: "o",
			Description:   "Output format.",
			Default:       "human",
			Value:         serpent.EnumOf(&output, "human", "json"),
		},
	)

	return cmd
}

type statusResponse struct {
	UnitName     string          `json:"unit_name"`
	Status       string          `json:"status"`
	IsReady      bool            `json:"is_ready"`
	Dependencies []dependencyRow `json:"dependencies"`
}

type dependencyRow struct {
	DependsOn      string `table:"Depends On,default_sort"`
	RequiredStatus string `table:"Required"`
	CurrentStatus  string `table:"Current"`
	IsSatisfied    bool   `table:"Satisfied"`
}

func outputJSON(w io.Writer, unitName string, statusResp agentsocket.SyncStatusResponse) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	dependencies := make([]dependencyRow, len(statusResp.Dependencies))
	for i, dep := range statusResp.Dependencies {
		dependencies[i] = dependencyRow{
			DependsOn:      dep.DependsOn,
			RequiredStatus: dep.RequiredStatus,
			CurrentStatus:  dep.CurrentStatus,
			IsSatisfied:    dep.IsSatisfied,
		}
	}

	if err := encoder.Encode(statusResponse{
		UnitName:     unitName,
		Status:       statusResp.Status,
		IsReady:      statusResp.IsReady,
		Dependencies: dependencies,
	}); err != nil {
		return xerrors.Errorf("encode status response: %w", err)
	}
	return nil
}

func outputHuman(w io.Writer, unitName string, statusResp agentsocket.SyncStatusResponse) error {
	cliui.Info(w, "Unit: %s", unitName)
	cliui.Info(w, "Status: %s", statusResp.Status)
	cliui.Info(w, "Ready: %t", strconv.FormatBool(statusResp.IsReady))

	if len(statusResp.Dependencies) == 0 {
		cliui.Info(w, "No dependencies")
		return nil
	}

	rows := make([]dependencyRow, len(statusResp.Dependencies))
	for i, dep := range statusResp.Dependencies {
		rows[i] = dependencyRow{
			DependsOn:      dep.DependsOn,
			RequiredStatus: dep.RequiredStatus,
			CurrentStatus:  dep.CurrentStatus,
			IsSatisfied:    dep.IsSatisfied,
		}
	}

	cliui.Info(w, "Dependencies:")
	rendered, err := cliui.DisplayTable(rows, "", nil)
	if err != nil {
		return xerrors.Errorf("render dependencies table: %w", err)
	}
	_, _ = fmt.Fprintln(w, rendered)

	return nil
}
