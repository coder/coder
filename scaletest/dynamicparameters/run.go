package dynamicparameters

import (
	"context"
	"fmt"
	"io"
	"slices"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/websocket"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config
}

var _ harness.Runnable = &Runner{}

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
	}
}

// Run executes the dynamic parameters test, which:
//
// 1. connects to the dynamic parameters stream
// 2. waits for the initial response
// 3. sends a change request
// 4. waits for the change response
// 5. closes the stream
func (r *Runner) Run(ctx context.Context, _ string, logs io.Writer) (retErr error) {
	startTime := time.Now()
	stream, err := r.client.TemplateVersionDynamicParameters(ctx, codersdk.Me, r.cfg.TemplateVersion)
	if err != nil {
		return xerrors.Errorf("connect to dynamic parameters stream: %w", err)
	}
	defer stream.Close(websocket.StatusNormalClosure)
	respCh := stream.Chan()

	var initTime time.Time
	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp, ok := <-respCh:
		if !ok {
			return xerrors.Errorf("dynamic parameters stream closed before initial response")
		}
		initTime = time.Now()
		r.cfg.Metrics.LatencyInitialResponseSeconds.
			WithLabelValues(r.cfg.MetricLabelValues...).
			Observe(initTime.Sub(startTime).Seconds())
		_, _ = fmt.Fprintf(logs, "initial response: %+v\n", resp)
		if !slices.ContainsFunc(resp.Parameters, func(p codersdk.PreviewParameter) bool {
			return p.Name == "zero"
		}) {
			return xerrors.Errorf("missing expected parameter: 'zero'")
		}
		if err := checkNoDiagnostics(resp); err != nil {
			return xerrors.Errorf("unexpected initial response diagnostics: %w", err)
		}
	}

	err = stream.Send(codersdk.DynamicParametersRequest{
		ID: 1,
		Inputs: map[string]string{
			"zero": "B",
		},
	})
	if err != nil {
		return xerrors.Errorf("send change request: %w", err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp, ok := <-respCh:
		if !ok {
			return xerrors.Errorf("dynamic parameters stream closed before change response")
		}
		_, _ = fmt.Fprintf(logs, "change response: %+v\n", resp)
		r.cfg.Metrics.LatencyChangeResponseSeconds.
			WithLabelValues(r.cfg.MetricLabelValues...).
			Observe(time.Since(initTime).Seconds())
		if resp.ID != 1 {
			return xerrors.Errorf("unexpected response ID: %d", resp.ID)
		}
		if err := checkNoDiagnostics(resp); err != nil {
			return xerrors.Errorf("unexpected change response diagnostics: %w", err)
		}
		return nil
	}
}

func checkNoDiagnostics(resp codersdk.DynamicParametersResponse) error {
	if len(resp.Diagnostics) != 0 {
		return xerrors.Errorf("unexpected response diagnostics: %v", resp.Diagnostics)
	}
	for _, param := range resp.Parameters {
		if len(param.Diagnostics) != 0 {
			return xerrors.Errorf("unexpected parameter diagnostics for '%s': %v", param.Name, param.Diagnostics)
		}
	}
	return nil
}
