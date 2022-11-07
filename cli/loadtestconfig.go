package cli

import (
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/loadtest/agentconn"
	"github.com/coder/coder/loadtest/harness"
	"github.com/coder/coder/loadtest/placebo"
	"github.com/coder/coder/loadtest/workspacebuild"
)

// LoadTestConfig is the overall configuration for a call to `coder loadtest`.
type LoadTestConfig struct {
	Strategy LoadTestStrategy `json:"strategy"`
	Tests    []LoadTest       `json:"tests"`
	// Timeout sets a timeout for the entire test run, to control the timeout
	// for each individual run use strategy.timeout.
	Timeout httpapi.Duration `json:"timeout"`
}

type LoadTestStrategyType string

const (
	LoadTestStrategyTypeLinear     LoadTestStrategyType = "linear"
	LoadTestStrategyTypeConcurrent LoadTestStrategyType = "concurrent"
)

type LoadTestStrategy struct {
	// Type is the type of load test strategy to use. Strategies determine how
	// to run tests concurrently.
	Type LoadTestStrategyType `json:"type"`

	// ConcurrencyLimit is the maximum number of concurrent runs. This only
	// applies if type == "concurrent". Negative values disable the concurrency
	// limit and attempts to perform all runs concurrently. The default value is
	// 100.
	ConcurrencyLimit int `json:"concurrency_limit"`

	// Shuffle determines whether or not to shuffle the test runs before
	// executing them.
	Shuffle bool `json:"shuffle"`
	// Timeout is the maximum amount of time to run each test for. This is
	// independent of the timeout specified in the test run. A timeout of 0
	// disables the timeout.
	Timeout httpapi.Duration `json:"timeout"`
}

func (s LoadTestStrategy) ExecutionStrategy() harness.ExecutionStrategy {
	var strategy harness.ExecutionStrategy
	switch s.Type {
	case LoadTestStrategyTypeLinear:
		strategy = harness.LinearExecutionStrategy{}
	case LoadTestStrategyTypeConcurrent:
		limit := s.ConcurrencyLimit
		if limit < 0 {
			return harness.ConcurrentExecutionStrategy{}
		}
		if limit == 0 {
			limit = 100
		}
		strategy = harness.ParallelExecutionStrategy{
			Limit: limit,
		}
	default:
		panic("unreachable, unknown strategy type " + s.Type)
	}

	if s.Timeout > 0 {
		strategy = harness.TimeoutExecutionStrategyWrapper{
			Timeout: time.Duration(s.Timeout),
			Inner:   strategy,
		}
	}
	if s.Shuffle {
		strategy = harness.ShuffleExecutionStrategyWrapper{
			Inner: strategy,
		}
	}

	return strategy
}

type LoadTestType string

const (
	LoadTestTypeAgentConn      LoadTestType = "agentconn"
	LoadTestTypePlacebo        LoadTestType = "placebo"
	LoadTestTypeWorkspaceBuild LoadTestType = "workspacebuild"
)

type LoadTest struct {
	// Type is the type of load test to run.
	Type LoadTestType `json:"type"`
	// Count is the number of test runs to execute with this configuration. If
	// the count is 0 or negative, defaults to 1.
	Count int `json:"count"`

	// AgentConn must be set if type == "agentconn".
	AgentConn *agentconn.Config `json:"agentconn,omitempty"`
	// Placebo must be set if type == "placebo".
	Placebo *placebo.Config `json:"placebo,omitempty"`
	// WorkspaceBuild must be set if type == "workspacebuild".
	WorkspaceBuild *workspacebuild.Config `json:"workspacebuild,omitempty"`
}

func (t LoadTest) NewRunner(client *codersdk.Client) (harness.Runnable, error) {
	switch t.Type {
	case LoadTestTypeAgentConn:
		if t.AgentConn == nil {
			return nil, xerrors.New("agentconn config must be set")
		}
		return agentconn.NewRunner(client, *t.AgentConn), nil
	case LoadTestTypePlacebo:
		if t.Placebo == nil {
			return nil, xerrors.New("placebo config must be set")
		}
		return placebo.NewRunner(*t.Placebo), nil
	case LoadTestTypeWorkspaceBuild:
		if t.WorkspaceBuild == nil {
			return nil, xerrors.Errorf("workspacebuild config must be set")
		}
		return workspacebuild.NewRunner(client, *t.WorkspaceBuild), nil
	default:
		return nil, xerrors.Errorf("unknown test type %q", t.Type)
	}
}

func (c *LoadTestConfig) Validate() error {
	err := c.Strategy.Validate()
	if err != nil {
		return xerrors.Errorf("validate strategy: %w", err)
	}

	for i, test := range c.Tests {
		err := test.Validate()
		if err != nil {
			return xerrors.Errorf("validate test %d: %w", i, err)
		}
	}

	return nil
}

func (s *LoadTestStrategy) Validate() error {
	switch s.Type {
	case LoadTestStrategyTypeLinear:
	case LoadTestStrategyTypeConcurrent:
	default:
		return xerrors.Errorf("invalid load test strategy type: %q", s.Type)
	}

	if s.Timeout < 0 {
		return xerrors.Errorf("invalid load test strategy timeout: %q", s.Timeout)
	}

	return nil
}

func (t *LoadTest) Validate() error {
	switch t.Type {
	case LoadTestTypeAgentConn:
		if t.AgentConn == nil {
			return xerrors.Errorf("agentconn test type must specify agentconn")
		}

		err := t.AgentConn.Validate()
		if err != nil {
			return xerrors.Errorf("validate agentconn: %w", err)
		}
	case LoadTestTypePlacebo:
		if t.Placebo == nil {
			return xerrors.Errorf("placebo test type must specify placebo")
		}

		err := t.Placebo.Validate()
		if err != nil {
			return xerrors.Errorf("validate placebo: %w", err)
		}
	case LoadTestTypeWorkspaceBuild:
		if t.WorkspaceBuild == nil {
			return xerrors.New("workspacebuild test type must specify workspacebuild")
		}

		err := t.WorkspaceBuild.Validate()
		if err != nil {
			return xerrors.Errorf("validate workspacebuild: %w", err)
		}
	default:
		return xerrors.Errorf("invalid load test type: %q", t.Type)
	}

	return nil
}
