package placebo
import (
	"errors"
	"github.com/coder/coder/v2/coderd/httpapi"
)
type Config struct {
	// Sleep is how long to sleep for. If unspecified, the test run will finish
	// instantly.
	Sleep httpapi.Duration `json:"sleep"`
	// Jitter is the maximum amount of jitter to add to the sleep duration. The
	// sleep value will be increased by a random value between 0 and jitter if
	// jitter is greater than 0.
	Jitter httpapi.Duration `json:"jitter"`
	// FailureChance is the chance that the test will fail. The value must be
	// between 0 and 1.
	FailureChance float64 `json:"failure_chance"`
}
func (c Config) Validate() error {
	if c.Sleep < 0 {
		return errors.New("sleep must be set to a positive value")
	}
	if c.Jitter < 0 {
		return errors.New("jitter must be set to a positive value")
	}
	if c.Jitter > 0 && c.Sleep == 0 {
		return errors.New("jitter must be 0 if sleep is 0")
	}
	if c.FailureChance < 0 || c.FailureChance > 1 {
		return errors.New("failure_chance must be between 0 and 1")
	}
	return nil
}
