package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/coder/coder/v2/scripts/cdev/passthrough"
	"github.com/coder/coder/v2/scripts/cdev/workingdir"
	"github.com/coder/serpent"
)

func main() {
	cmd := &serpent.Command{
		Use:        "cdev",
		Short:      "Development environment manager for Coder",
		Long:       "A smart, opinionated tool for running the Coder development stack.",
		Middleware: workingdir.WorkingContext,
		Children:   []*serpent.Command{
			//pprofCmd(),
		},
		Handler: passthrough.DockerComposePassthroughCmd,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	// We want to catch SIGINT (Ctrl+C) and SIGTERM (graceful shutdown).
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs

		// Notify the main function that cleanup is finished.
		// TODO: Would be best to call a `Close()` function and try a graceful shutdown first, but this is good enough for now.
		cancel()
	}()

	err := cmd.Invoke().WithContext(ctx).WithOS().Run()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

//func pprofCmd() *serpent.Command {
//	var instance int64
//	return &serpent.Command{
//		Use:   "pprof <profile>",
//		Short: "Open pprof web UI for a running coderd instance",
//		Long: `Open the pprof web UI for a running coderd instance.
//
//Supported profiles:
//  profile      CPU profile (30s sample)
//  heap         Heap memory allocations
//  goroutine    Stack traces of all goroutines
//  allocs       Past memory allocations
//  block        Stack traces of blocking operations
//  mutex        Stack traces of mutex contention
//  threadcreate Stack traces that led to new OS threads
//  trace        Execution trace (30s sample)
//
//Examples:
//  cdev pprof heap
//  cdev pprof profile
//  cdev pprof goroutine
//  cdev pprof -i 1 heap     # instance 1`,
//		Options: serpent.OptionSet{
//			{
//				Name:          "Instance",
//				Description:   "Coderd instance index (0-based).",
//				Flag:          "instance",
//				FlagShorthand: "i",
//				Default:       "0",
//				Value:         serpent.Int64Of(&instance),
//			},
//		},
//		Handler: func(inv *serpent.Invocation) error {
//			if len(inv.Args) != 1 {
//				_ = serpent.DefaultHelpFn()(inv)
//				return xerrors.New("expected exactly one argument: the profile name")
//			}
//			profile := inv.Args[0]
//
//			url := fmt.Sprintf("http://localhost:%d/debug/pprof/%s", catalog.PprofPortNum(int(instance)), profile)
//			if profile == "profile" || profile == "trace" {
//				url += "?seconds=30"
//			}
//
//			_, _ = fmt.Fprintf(inv.Stdout, "Opening pprof web UI for instance %d, %q at %s\n", instance, profile, url)
//
//			//nolint:gosec // User-provided profile name is passed as a URL path.
//			cmd := exec.CommandContext(inv.Context(), "go", "tool", "pprof", "-http=:", url)
//			cmd.Stdout = inv.Stdout
