package placebo
import (
	"errors"
	"context"
	"fmt"
	"io"
	"math/rand"
	"time"
	"github.com/coder/coder/v2/scaletest/harness"
)
type Runner struct {
	cfg Config
}
var _ harness.Runnable = &Runner{}
// NewRunner creates a new placebo loadtest Runner. The test will sleep for the
// specified duration if set, and will add a random amount of jitter between 0
// and the specified jitter value if set.
func NewRunner(cfg Config) *Runner {
	return &Runner{
		cfg: cfg,
	}
}
// Run implements Runnable.
func (r *Runner) Run(ctx context.Context, _ string, logs io.Writer) error {
	sleepDur := time.Duration(r.cfg.Sleep)
	if r.cfg.Jitter > 0 {
		//nolint:gosec // not used for crypto
		sleepDur += time.Duration(rand.Int63n(int64(r.cfg.Jitter)))
		// This makes it easier to tell if jitter was applied in tests.
		sleepDur += time.Millisecond
	}
	if sleepDur > 0 {
		_, _ = fmt.Fprintf(logs, "sleeping for %s\n", sleepDur)
		t := time.NewTimer(sleepDur)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	if r.cfg.FailureChance > 0 {
		_, _ = fmt.Fprintf(logs, "failure chance is %f\n", r.cfg.FailureChance)
		_, _ = fmt.Fprintln(logs, "rolling the dice of fate...")
		//nolint:gosec // not used for crypto
		roll := rand.Float64()
		_, _ = fmt.Fprintf(logs, "rolled: %f\n", roll)
		if roll < r.cfg.FailureChance {
			_, _ = fmt.Fprintln(logs, ":(")
			return errors.New("test failed due to configured failure chance")
		}
	}
	return nil
}
