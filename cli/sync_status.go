package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/agent/agentsocket"
)

type outputFormat string

const (
	outputFormatHuman outputFormat = "human"
	outputFormatJSON  outputFormat = "json"
	outputFormatDOT   outputFormat = "dot"
)

func (r *RootCmd) syncStatus() *serpent.Command {
	var output string

	cmd := &serpent.Command{
		Use:   "status <unit>",
		Short: "Inspect service status and dependency state",
		Long:  "Display the current status of a service unit, its readiness state, and detailed information about its dependencies. Shows which dependencies are satisfied and which are still pending. Supports multiple output formats (human-readable, JSON, or DOT graph) for integration with other tools or visualization.",
		Handler: func(i *serpent.Invocation) error {
			ctx := context.Background()

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
				return outputJSON(unit, statusResp)
			case outputFormatDOT:
				return outputDOT(statusResp)
			default: // outputFormatHuman
				return outputHuman(unit, statusResp)
			}
		},
	}

	cmd.Options = append(cmd.Options,
		serpent.Option{
			Flag:          "output",
			FlagShorthand: "o",
			Description:   "Output format: human, json, or dot.",
			Value:         serpent.EnumOf(&output, "human", "json", "dot"),
		},
	)

	return cmd
}

func outputJSON(unitName string, statusResp agentsocket.SyncStatusResponse) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(statusResp)
}

func outputDOT(statusResp agentsocket.SyncStatusResponse) error {
	return xerrors.New("DOT output is not currently supported")
}

func outputHuman(unitName string, statusResp agentsocket.SyncStatusResponse) error {
	fmt.Printf("Unit: %s\n", unitName)
	fmt.Printf("Status: %s\n", statusResp.Status)
	fmt.Printf("Ready: %t\n", statusResp.IsReady)
	fmt.Println()

	if len(statusResp.Dependencies) == 0 {
		fmt.Println("No dependencies")
		return nil
	}

	fmt.Println("Dependencies:")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("%-20s %-15s %-15s %-10s\n", "Depends On", "Required", "Current", "Satisfied")
	fmt.Println(strings.Repeat("-", 80))

	for _, dep := range statusResp.Dependencies {
		satisfied := "✓"
		if !dep.IsSatisfied {
			satisfied = "✗"
		}
		fmt.Printf("%-20s %-15s %-15s %-10s\n",
			dep.DependsOn,
			dep.RequiredStatus,
			dep.CurrentStatus,
			satisfied,
		)
	}

	return nil
}
