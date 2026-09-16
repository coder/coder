package lab

import (
	"encoding/json"
	"fmt"
	"math"
)

// Knob describes one integer parameter a scenario accepts on Start. Unit
// is a short display label such as "MiB/s".
type Knob struct {
	Name    string `json:"name"`
	Unit    string `json:"unit"`
	Default int    `json:"default"`
	Min     int    `json:"min"`
	Max     int    `json:"max"`
}

// KnobError reports a knob value that is missing, malformed, unknown, or
// outside the range the scenario accepts.
type KnobError struct {
	Knob   string
	Value  any
	Reason string
}

func (e *KnobError) Error() string {
	return fmt.Sprintf("knob %q: %s (got %v)", e.Knob, e.Reason, e.Value)
}

// clampKnob resolves the value for spec from knobs. A missing knob yields
// spec.Default. A present knob must be an integer within [spec.Min,
// spec.Max]; anything else returns a *KnobError. Out-of-range values are
// rejected rather than silently clamped so a caller learns that its
// request was not honored.
func clampKnob(knobs map[string]any, spec Knob) (int, error) {
	raw, ok := knobs[spec.Name]
	if !ok || raw == nil {
		return spec.Default, nil
	}
	v, err := knobInt(raw)
	if err != nil {
		return 0, &KnobError{Knob: spec.Name, Value: raw, Reason: err.Error()}
	}
	if v < spec.Min || v > spec.Max {
		return 0, &KnobError{
			Knob:   spec.Name,
			Value:  raw,
			Reason: fmt.Sprintf("must be between %d and %d", spec.Min, spec.Max),
		}
	}
	return v, nil
}

// parseKnobs resolves every knob in specs and rejects keys in knobs that no
// spec describes.
func parseKnobs(knobs map[string]any, specs []Knob) (map[string]int, error) {
	known := make(map[string]struct{}, len(specs))
	out := make(map[string]int, len(specs))
	for _, spec := range specs {
		known[spec.Name] = struct{}{}
		v, err := clampKnob(knobs, spec)
		if err != nil {
			return nil, err
		}
		out[spec.Name] = v
	}
	for name, raw := range knobs {
		if _, ok := known[name]; !ok {
			return nil, &KnobError{Knob: name, Value: raw, Reason: "unknown knob"}
		}
	}
	return out, nil
}

// knobInt converts the value types produced by encoding/json and by Go
// callers into an int. Floats must be integral.
func knobInt(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		if v != math.Trunc(v) || math.IsInf(v, 0) || math.IsNaN(v) {
			return 0, fmt.Errorf("must be an integer")
		}
		if v > math.MaxInt32 || v < math.MinInt32 {
			return 0, fmt.Errorf("out of integer range")
		}
		return int(v), nil
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, fmt.Errorf("must be an integer")
		}
		return int(n), nil
	default:
		return 0, fmt.Errorf("must be a number, not %T", raw)
	}
}
