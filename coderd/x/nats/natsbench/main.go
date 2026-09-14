package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
)

func main() {
	// Cancel running scenarios on SIGINT/SIGTERM so the per-phase
	// selects unwind cleanly instead of requiring kill -9. stop() is
	// called explicitly rather than deferred so os.Exit below does not
	// skip it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := runCLI(ctx, os.Args, os.Stdout, os.Stderr)
	stop()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "natsbench: %v\n", err)
		os.Exit(1)
	}
}

// runCLI runs the natsbench command-line interface. args is the full
// argument vector (args[0] is the program name). The grouped markdown
// report is written to stdout and progress and logs to stderr. With no
// flags it runs the default scenario matrix; -scenario runs one named
// scenario; any custom shape flag runs a single custom configuration.
// It returns an error when the flags are invalid or any run fails.
func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		run     cliRun
		list    = fs.Bool("list", false, "list default scenarios and exit")
		verbose = fs.Bool("v", false, "enable debug logging")
	)
	fs.StringVar(&run.scenarioName, "scenario", "", "run one named default scenario (see -list)")
	fs.IntVar(&run.messages, "messages", 0, "total messages across all publishers (0 keeps scenario defaults, or 100000 for custom runs)")
	fs.IntVar(&run.payload, "payload", Payload8KB, "payload size in bytes (custom run)")
	fs.IntVar(&run.subjects, "subjects", 10, "number of subjects (custom run)")
	fs.IntVar(&run.publishers, "publishers", 10, "number of publishers (custom run)")
	fs.IntVar(&run.subscribers, "subscribers", 50, "number of subscribers (custom run)")
	fs.IntVar(&run.replicas, "replicas", 1, "number of embedded pubsub nodes (custom run)")
	fs.IntVar(&run.publishConns, "publish-conns", DefaultConns, "publisher connection pool size (applies to every run)")
	fs.IntVar(&run.subscribeConns, "subscribe-conns", DefaultConns, "subscriber connection pool size (applies to every run)")
	fs.Int64Var(&run.seed, "seed", DefaultSeed, "seed for pseudorandom node placement (applies to every run); same seed reproduces the same placement")
	fs.DurationVar(&run.timeout, "timeout", 2*time.Minute, "per-phase timeout")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "payload", "subjects", "publishers", "subscribers", "replicas":
			run.shapeFlagSet = true
		}
	})

	if *list {
		for _, sc := range DefaultScenarios() {
			c := sc.Config
			_, _ = fmt.Fprintf(stdout, "%s: messages=%d payload=%d subjects=%d publishers=%d subscribers=%d replicas=%d\n",
				sc.Name, c.Messages, c.PayloadSize, c.Subjects, c.Publishers, c.Subscribers, c.Replicas)
		}
		return nil
	}

	scenarios, err := run.scenarios()
	if err != nil {
		return err
	}

	level := slog.LevelWarn
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.Make(sloghuman.Sink(stderr)).Leveled(level)

	// Scenarios run sequentially so they never compete for CPU, memory,
	// or the network stack and skew each other's numbers.
	results := make([]ScenarioResult, 0, len(scenarios))
	failed := false
	for _, sc := range scenarios {
		// Stop launching scenarios once interrupted, rather than
		// letting each remaining Run fail with a confusing topology
		// error from the canceled context.
		if err := ctx.Err(); err != nil {
			return xerrors.Errorf("interrupted before %s: %w", sc.Name, err)
		}
		_, _ = fmt.Fprintf(stderr, "running %s...\n", sc.Name)
		res, runErr := Run(ctx, logger, sc.Config)
		results = append(results, ScenarioResult{Scenario: sc, Result: res, Err: runErr})
		if runErr != nil {
			failed = true
			_, _ = fmt.Fprintf(stderr, "%s failed: %v\n", sc.Name, runErr)
			continue
		}
		_, _ = fmt.Fprintf(stderr, "%s: pubs/sec=%.0f deliveries/sec=%.0f\n",
			sc.Name, res.PubsPerSec, res.DeliveriesPerSec)
	}

	if err := RenderMarkdown(stdout, results); err != nil {
		return xerrors.Errorf("render report: %w", err)
	}
	if failed {
		return xerrors.New("one or more runs failed; see report")
	}
	return nil
}

// cliRun holds the parsed flag values that select the scenarios to run.
type cliRun struct {
	scenarioName string
	// shapeFlagSet is true when any custom-shape flag was passed, which
	// selects a single custom run.
	shapeFlagSet   bool
	messages       int
	payload        int
	subjects       int
	publishers     int
	subscribers    int
	replicas       int
	publishConns   int
	subscribeConns int
	seed           int64
	timeout        time.Duration
}

// scenarios resolves the parsed flags into the scenarios to run: one
// named default scenario, a single custom shape when any shape flag was
// set, or the full default matrix. The per-phase timeout is applied to
// every scenario, and -messages overrides the message count when set.
func (c cliRun) scenarios() ([]Scenario, error) {
	switch {
	case c.scenarioName != "":
		if c.shapeFlagSet {
			return nil, xerrors.New("-scenario and custom shape flags are mutually exclusive")
		}
		for _, sc := range DefaultScenarios() {
			if sc.Name != c.scenarioName {
				continue
			}
			if c.messages > 0 {
				sc.Config.Messages = c.messages
			}
			sc.Config.PublishConns = c.publishConns
			sc.Config.SubscribeConns = c.subscribeConns
			sc.Config.Seed = c.seed
			sc.Config.Timeout = c.timeout
			return []Scenario{sc}, nil
		}
		return nil, xerrors.Errorf("unknown scenario %q; use -list to see available scenarios", c.scenarioName)
	case c.shapeFlagSet:
		messages := c.messages
		if messages <= 0 {
			messages = DefaultMessages
		}
		return []Scenario{{
			Name: "custom",
			Config: Config{
				Messages:       messages,
				PayloadSize:    c.payload,
				Subjects:       c.subjects,
				Publishers:     c.publishers,
				Subscribers:    c.subscribers,
				Replicas:       c.replicas,
				PublishConns:   c.publishConns,
				SubscribeConns: c.subscribeConns,
				Seed:           c.seed,
				Timeout:        c.timeout,
			},
		}}, nil
	default:
		scenarios := DefaultScenarios()
		for i := range scenarios {
			if c.messages > 0 {
				scenarios[i].Config.Messages = c.messages
			}
			scenarios[i].Config.PublishConns = c.publishConns
			scenarios[i].Config.SubscribeConns = c.subscribeConns
			scenarios[i].Config.Seed = c.seed
			scenarios[i].Config.Timeout = c.timeout
		}
		return scenarios, nil
	}
}
