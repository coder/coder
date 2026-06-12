// Command natsbench benchmarks Coder's NATS-backed pubsub. With no
// flags it runs the default scenario matrix; -scenario runs one named
// scenario; shape flags (-payload, -subjects, -publishers,
// -subscribers, -replicas) run a single custom configuration. The
// grouped markdown report is written to stdout and logs to stderr.
//
// The benchmarks are heavyweight by design; this command is the only
// way to run them, so they can never run in CI.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/x/nats/natsbench"
)

// options carries the parsed flag values.
type options struct {
	scenarioName string
	messages     int
	payload      int
	subjects     int
	publishers   int
	subscribers  int
	replicas     int
	timeout      time.Duration
	verbose      bool
	// shapeFlagSet records whether any custom-shape flag was passed
	// explicitly, which selects a single custom run.
	shapeFlagSet bool
}

func main() {
	var (
		opts options
		list bool
	)
	flag.StringVar(&opts.scenarioName, "scenario", "", "run one named default scenario (see -list)")
	flag.BoolVar(&list, "list", false, "list default scenarios and exit")
	flag.IntVar(&opts.messages, "messages", 0, "total messages across all publishers (0 keeps scenario defaults, or 100000 for custom runs)")
	flag.IntVar(&opts.payload, "payload", natsbench.Payload8KB, "payload size in bytes (custom run)")
	flag.IntVar(&opts.subjects, "subjects", 10, "number of subjects (custom run)")
	flag.IntVar(&opts.publishers, "publishers", 10, "number of publishers (custom run)")
	flag.IntVar(&opts.subscribers, "subscribers", 50, "number of subscribers (custom run)")
	flag.IntVar(&opts.replicas, "replicas", 1, "number of embedded pubsub nodes (custom run)")
	flag.DurationVar(&opts.timeout, "timeout", 2*time.Minute, "per-phase timeout")
	flag.BoolVar(&opts.verbose, "v", false, "enable debug logging")
	flag.Parse()
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "payload", "subjects", "publishers", "subscribers", "replicas":
			opts.shapeFlagSet = true
		}
	})

	if list {
		for _, sc := range natsbench.DefaultScenarios() {
			cfg := sc.Config
			_, _ = fmt.Printf("%s: messages=%d payload=%d subjects=%d publishers=%d subscribers=%d replicas=%d\n",
				sc.Name, cfg.Messages, cfg.PayloadSize, cfg.Subjects, cfg.Publishers, cfg.Subscribers, cfg.Replicas)
		}
		return
	}

	if err := run(opts); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "natsbench: %v\n", err)
		os.Exit(1)
	}
}

func run(opts options) error {
	level := slog.LevelWarn
	if opts.verbose {
		level = slog.LevelDebug
	}
	logger := slog.Make(sloghuman.Sink(os.Stderr)).Leveled(level)

	scenarios, err := selectScenarios(opts)
	if err != nil {
		return err
	}

	// Scenarios run sequentially so they never compete for CPU, memory,
	// or the network stack and skew each other's numbers.
	results := make([]natsbench.ScenarioResult, 0, len(scenarios))
	failed := false
	for _, sc := range scenarios {
		_, _ = fmt.Fprintf(os.Stderr, "running %s...\n", sc.Name)
		res, err := natsbench.Run(context.Background(), logger, sc.Config)
		results = append(results, natsbench.ScenarioResult{Scenario: sc, Result: res, Err: err})
		switch {
		case err != nil:
			failed = true
			_, _ = fmt.Fprintf(os.Stderr, "%s failed: %v\n", sc.Name, err)
		default:
			_, _ = fmt.Fprintf(os.Stderr, "%s: pubs/sec=%.0f deliveries/sec=%.0f\n",
				sc.Name, res.PubsPerSec, res.DeliveriesPerSec)
		}
	}

	if err := natsbench.RenderMarkdown(os.Stdout, results); err != nil {
		return xerrors.Errorf("render report: %w", err)
	}
	if failed {
		return xerrors.New("one or more runs failed; see report")
	}
	return nil
}

// selectScenarios resolves the parsed flags into the scenarios to run:
// one named default scenario, a single custom shape when any shape flag
// was set, or the full default matrix.
func selectScenarios(opts options) ([]natsbench.Scenario, error) {
	switch {
	case opts.scenarioName != "":
		if opts.shapeFlagSet {
			return nil, xerrors.New("-scenario and custom shape flags are mutually exclusive")
		}
		for _, sc := range natsbench.DefaultScenarios() {
			if sc.Name != opts.scenarioName {
				continue
			}
			if opts.messages > 0 {
				sc.Config.Messages = opts.messages
			}
			sc.Config.Timeout = opts.timeout
			return []natsbench.Scenario{sc}, nil
		}
		return nil, xerrors.Errorf("unknown scenario %q; use -list to see available scenarios", opts.scenarioName)
	case opts.shapeFlagSet:
		messages := opts.messages
		if messages <= 0 {
			messages = natsbench.DefaultMessages
		}
		return []natsbench.Scenario{{
			Name: "custom",
			Config: natsbench.Config{
				Messages:    messages,
				PayloadSize: opts.payload,
				Subjects:    opts.subjects,
				Publishers:  opts.publishers,
				Subscribers: opts.subscribers,
				Replicas:    opts.replicas,
				Timeout:     opts.timeout,
			},
		}}, nil
	default:
		scenarios := natsbench.DefaultScenarios()
		for i := range scenarios {
			if opts.messages > 0 {
				scenarios[i].Config.Messages = opts.messages
			}
			scenarios[i].Config.Timeout = opts.timeout
		}
		return scenarios, nil
	}
}
