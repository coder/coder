package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/codersdk/agentsdk"
)

type outputFormat string

const (
	outputFormatHuman outputFormat = "human"
	outputFormatJSON  outputFormat = "json"
	outputFormatDOT   outputFormat = "dot"
)

func (r *RootCmd) syncStatus() *serpent.Command {
	var (
		output    string
		recursive bool
	)

	cmd := &serpent.Command{
		Use:   "status <unit>",
		Short: "Show the status of a unit and its dependencies",
		Long:  "Display the current status of a unit and information about its dependencies. Supports multiple output formats.",
		Handler: func(i *serpent.Invocation) error {
			ctx := context.Background()

			if len(i.Args) != 1 {
				return xerrors.New("exactly one unit name is required")
			}
			unit := i.Args[0]

			// Connect to agent socket
			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: "/tmp/coder.sock",
			})
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			// Get status information
			statusResp, err := client.SyncStatus(ctx, unit, recursive)
			if err != nil {
				return xerrors.Errorf("get status failed: %w", err)
			}

			// Output based on format
			switch outputFormat(output) {
			case outputFormatJSON:
				return outputJSON(statusResp)
			case outputFormatDOT:
				return outputDOT(statusResp)
			default: // outputFormatHuman
				return outputHuman(statusResp)
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
		serpent.Option{
			Flag:          "recursive",
			FlagShorthand: "r",
			Description:   "Show transitive dependencies and include DOT graph.",
			Value:         serpent.BoolOf(&recursive),
		},
	)

	return cmd
}

func outputJSON(statusResp *agentsdk.SyncStatusResponse) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(statusResp)
}

func outputDOT(statusResp *agentsdk.SyncStatusResponse) error {
	if statusResp.DOT == "" {
		return xerrors.New("DOT output requires --recursive flag")
	}
	fmt.Println(statusResp.DOT)
	return nil
}

func outputHuman(statusResp *agentsdk.SyncStatusResponse) error {
	// Unit status
	fmt.Printf("Unit: %s\n", statusResp.Unit)
	fmt.Printf("Status: %s\n", statusResp.Status)
	fmt.Printf("Ready: %t\n", statusResp.IsReady)
	fmt.Println()

	// Dependencies
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
