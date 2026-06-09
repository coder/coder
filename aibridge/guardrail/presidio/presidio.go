// Package presidio implements a guardrail backed by Microsoft Presidio's
// analyzer and anonymizer services. It detects PII entities in the request's
// text spans and, per configured entity action, either blocks the request or
// masks the offending spans in place.
package presidio

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/guardrail"
)

// EntityAction is what to do when a configured entity type is detected.
type EntityAction string

const (
	// ActionMask redacts the detected span in the request body.
	ActionMask EntityAction = "MASK"
	// ActionBlock rejects the whole request.
	ActionBlock EntityAction = "BLOCK"
)

// defaultRedactValue replaces masked spans. Presidio's anonymizer substitutes
// it for every detected entity span.
const defaultRedactValue = "<REDACTED>"

// Config configures a Presidio guardrail.
type Config struct {
	// Name identifies the guardrail (namespaces annotations, labels metrics).
	Name string
	// AnalyzerURL is the base URL of the Presidio analyzer service. The adapter
	// POSTs to AnalyzerURL + "/analyze".
	AnalyzerURL string
	// AnonymizerURL is the base URL of the Presidio anonymizer service. Only
	// required when at least one entity uses ActionMask. The adapter POSTs to
	// AnonymizerURL + "/anonymize".
	AnonymizerURL string
	// Language is the analyzer language code (default "en").
	Language string
	// EntityActions maps a Presidio entity type (e.g. "EMAIL_ADDRESS") to the
	// action taken when it is detected. Entity types absent from the map are
	// not requested from the analyzer.
	EntityActions map[string]EntityAction
	// ScoreThresholds is the per-entity minimum confidence score to act on a
	// detection. Entities without an entry use DefaultThreshold.
	ScoreThresholds map[string]float64
	// DefaultThreshold is the fallback minimum confidence score (default 0,
	// i.e. act on any detection).
	DefaultThreshold float64
	// RedactValue replaces masked spans (default "<REDACTED>").
	RedactValue string
	// HTTPClient is used for both services; defaults to http.DefaultClient.
	HTTPClient *http.Client
}

// Guardrail is a Presidio-backed [guardrail.Guardrail].
type Guardrail struct {
	cfg      Config
	entities []string
	client   *http.Client
}

var _ guardrail.Guardrail = (*Guardrail)(nil)

// jsonConfig is the persisted JSON shape of a Presidio guardrail's config.
type jsonConfig struct {
	AnalyzerURL      string                  `json:"analyzer_url"`
	AnonymizerURL    string                  `json:"anonymizer_url"`
	Language         string                  `json:"language"`
	EntityActions    map[string]EntityAction `json:"entity_actions"`
	ScoreThresholds  map[string]float64      `json:"score_thresholds"`
	DefaultThreshold float64                 `json:"default_threshold"`
	RedactValue      string                  `json:"redact_value"`
}

// NewFromConfig builds a Presidio guardrail from its persisted JSON config.
// Presidio requires no credential, so the secret is unused. It is the shared
// entry point for both config validation (API) and runtime construction
// (loader).
func NewFromConfig(name string, raw []byte) (*Guardrail, error) {
	var c jsonConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, xerrors.Errorf("parse presidio config: %w", err)
	}
	return New(Config{
		Name:             name,
		AnalyzerURL:      c.AnalyzerURL,
		AnonymizerURL:    c.AnonymizerURL,
		Language:         c.Language,
		EntityActions:    c.EntityActions,
		ScoreThresholds:  c.ScoreThresholds,
		DefaultThreshold: c.DefaultThreshold,
		RedactValue:      c.RedactValue,
	})
}

// New validates cfg and returns a ready guardrail.
func New(cfg Config) (*Guardrail, error) {
	if cfg.Name == "" {
		return nil, xerrors.New("presidio: name required")
	}
	if cfg.AnalyzerURL == "" {
		return nil, xerrors.New("presidio: analyzer URL required")
	}
	if len(cfg.EntityActions) == 0 {
		return nil, xerrors.New("presidio: at least one entity action required")
	}

	entities := make([]string, 0, len(cfg.EntityActions))
	needAnonymizer := false
	for e, a := range cfg.EntityActions {
		switch a {
		case ActionMask:
			needAnonymizer = true
		case ActionBlock:
		default:
			return nil, xerrors.Errorf("presidio: unknown action %q for entity %q", a, e)
		}
		entities = append(entities, e)
	}
	sort.Strings(entities) // stable request payload + deterministic tests
	if needAnonymizer && cfg.AnonymizerURL == "" {
		return nil, xerrors.New("presidio: anonymizer URL required when an entity uses MASK")
	}

	if cfg.Language == "" {
		cfg.Language = "en"
	}
	if cfg.RedactValue == "" {
		cfg.RedactValue = defaultRedactValue
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Guardrail{cfg: cfg, entities: entities, client: client}, nil
}

// Name implements guardrail.Guardrail.
func (g *Guardrail) Name() string { return g.cfg.Name }

// Evaluate analyzes every extracted text span. A BLOCK-action detection rejects
// the request; MASK-action detections produce body edits redacting the spans.
// Detected entity counts are always returned as annotations.
func (g *Guardrail) Evaluate(ctx context.Context, req guardrail.Request) (guardrail.Result, error) {
	var (
		res    guardrail.Result
		counts = map[string]int{}
	)

	for _, ref := range req.Texts {
		if ref.Value == "" {
			continue
		}
		detections, err := g.analyze(ctx, ref.Value)
		if err != nil {
			return guardrail.Result{}, err
		}

		var mask []detection
		blockedHere := false
		for _, d := range detections {
			action, ok := g.cfg.EntityActions[d.EntityType]
			if !ok || d.Score < g.threshold(d.EntityType) {
				continue
			}
			counts[d.EntityType]++
			switch action {
			case ActionBlock:
				blockedHere = true
			case ActionMask:
				mask = append(mask, d)
			}
		}

		if blockedHere {
			res.Action = guardrail.ActionBlock
			if res.Reason == "" {
				res.Reason = "request contains disallowed sensitive data"
			}
			// The request will be rejected; no point masking it.
			continue
		}

		// Skip anonymization once a block is decided elsewhere; edits are moot.
		if res.Action == guardrail.ActionBlock || len(mask) == 0 {
			continue
		}
		masked, err := g.anonymize(ctx, ref.Value, mask)
		if err != nil {
			return guardrail.Result{}, err
		}
		res.Edits = append(res.Edits, guardrail.Edit{Pointer: ref.Pointer, Value: masked})
	}

	if len(counts) > 0 {
		res.Annotations = map[string]any{
			"entities": counts,
			"blocked":  res.Action == guardrail.ActionBlock,
		}
	}
	return res, nil
}

func (g *Guardrail) threshold(entity string) float64 {
	if t, ok := g.cfg.ScoreThresholds[entity]; ok {
		return t
	}
	return g.cfg.DefaultThreshold
}

// detection is one analyzer result.
type detection struct {
	EntityType string  `json:"entity_type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Score      float64 `json:"score"`
}

func (g *Guardrail) analyze(ctx context.Context, text string) ([]detection, error) {
	body := map[string]any{
		"text":     text,
		"language": g.cfg.Language,
		"entities": g.entities,
	}
	var out []detection
	if err := g.post(ctx, g.cfg.AnalyzerURL, "/analyze", body, &out); err != nil {
		return nil, xerrors.Errorf("presidio analyze: %w", err)
	}
	return out, nil
}

func (g *Guardrail) anonymize(ctx context.Context, text string, results []detection) (string, error) {
	body := map[string]any{
		"text":             text,
		"analyzer_results": results,
		"anonymizers": map[string]any{
			"DEFAULT": map[string]any{
				"type":      "replace",
				"new_value": g.cfg.RedactValue,
			},
		},
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := g.post(ctx, g.cfg.AnonymizerURL, "/anonymize", body, &out); err != nil {
		return "", xerrors.Errorf("presidio anonymize: %w", err)
	}
	return out.Text, nil
}

func (g *Guardrail) post(ctx context.Context, base, path string, in, out any) error {
	payload, err := json.Marshal(in)
	if err != nil {
		return xerrors.Errorf("marshal request: %w", err)
	}
	url := strings.TrimRight(base, "/") + path
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return xerrors.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return xerrors.Errorf("unexpected status %s: %s", resp.Status, strings.TrimSpace(string(snippet)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return xerrors.Errorf("decode response: %w", err)
	}
	return nil
}
