// Package bedrock implements a guardrail backed by AWS Bedrock's ApplyGuardrail
// API. It is the deliberately awkward adapter that stresses the guardrail
// abstraction along three axes the HTTP-bearer vendors do not exercise:
//
//   - Auth is SigV4 request signing, not a static bearer header, so it cannot
//     reuse a shared "POST with an Authorization header" helper. Its credential
//     is a multi-field blob (access key, secret, optional session token), not a
//     single token string.
//   - The response does not map maskings back per input span: ApplyGuardrail
//     returns the masked text as a single collapsed output plus a structured
//     assessments block, so the adapter scans one span (the latest user prompt)
//     and writes the whole masked value back to that one pointer.
//   - Block vs mask is not a field on the response; both surface as
//     action=GUARDRAIL_INTERVENED. The adapter must inspect the assessments to
//     tell an ANONYMIZED (mask) outcome from a BLOCKED filter/topic/word
//     outcome.
//
// API: https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_ApplyGuardrail.html
package bedrock

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/guardrail"
)

// service is the SigV4 service name for the Bedrock runtime.
const service = "bedrock"

// Config configures a Bedrock guardrail. The AWS credential is supplied
// separately (the dbcrypt-encrypted blob), not here.
type Config struct {
	// Name identifies the guardrail (namespaces annotations, labels metrics).
	Name string
	// Region is the AWS region of the guardrail (e.g. "us-east-1").
	Region string
	// GuardrailIdentifier is the Bedrock guardrail id or ARN.
	GuardrailIdentifier string
	// GuardrailVersion is the guardrail version (e.g. "DRAFT" or "1").
	GuardrailVersion string
	// Endpoint overrides the derived bedrock-runtime endpoint (for testing or
	// VPC endpoints). Defaults to https://bedrock-runtime.{region}.amazonaws.com.
	Endpoint string
	// HTTPClient defaults to http.DefaultClient.
	HTTPClient *http.Client
	// now is injected in tests so signing time is deterministic.
	now func() time.Time
}

// credential is the decrypted multi-field AWS credential blob. A single
// credential string would not fit SigV4; the blob is stored JSON-encoded in the
// one dbcrypt-encrypted column.
type credential struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
}

// jsonConfig is the persisted JSON shape of a Bedrock guardrail's config.
type jsonConfig struct {
	Region              string `json:"region"`
	GuardrailIdentifier string `json:"guardrail_identifier"`
	GuardrailVersion    string `json:"guardrail_version"`
	Endpoint            string `json:"endpoint"`
}

// Guardrail is a Bedrock-backed [guardrail.Guardrail].
type Guardrail struct {
	cfg    Config
	cred   credential
	client *http.Client
	signer *v4.Signer
}

var _ guardrail.Guardrail = (*Guardrail)(nil)

// NewFromConfig builds a Bedrock guardrail from its persisted JSON config and
// decrypted credential (a JSON blob of AWS keys). It is the shared entry point
// for both config validation (API) and runtime construction.
func NewFromConfig(name string, raw []byte, cred string) (*Guardrail, error) {
	var c jsonConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, xerrors.Errorf("parse bedrock config: %w", err)
	}
	var parsed credential
	// An empty credential is permitted at validation time (the API validates
	// config shape without a secret); New enforces presence for runtime use only
	// when a credential is supplied.
	if strings.TrimSpace(cred) != "" {
		if err := json.Unmarshal([]byte(cred), &parsed); err != nil {
			return nil, xerrors.Errorf("parse bedrock credential: %w", err)
		}
	}
	return New(Config{
		Name:                name,
		Region:              c.Region,
		GuardrailIdentifier: c.GuardrailIdentifier,
		GuardrailVersion:    c.GuardrailVersion,
		Endpoint:            c.Endpoint,
	}, parsed)
}

// New validates cfg and returns a ready guardrail.
func New(cfg Config, cred credential) (*Guardrail, error) {
	if cfg.Name == "" {
		return nil, xerrors.New("bedrock: name required")
	}
	if cfg.Region == "" {
		return nil, xerrors.New("bedrock: region required")
	}
	if cfg.GuardrailIdentifier == "" {
		return nil, xerrors.New("bedrock: guardrail_identifier required")
	}
	if cfg.GuardrailVersion == "" {
		return nil, xerrors.New("bedrock: guardrail_version required")
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com", cfg.Region)
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Guardrail{cfg: cfg, cred: cred, client: client, signer: v4.NewSigner()}, nil
}

// Name implements guardrail.Guardrail.
func (g *Guardrail) Name() string { return g.cfg.Name }

// wireReq is the ApplyGuardrail request body.
type wireReq struct {
	Source  string        `json:"source"` // "INPUT"
	Content []contentItem `json:"content"`
}

type contentItem struct {
	Text textBlock `json:"text"`
}

type textBlock struct {
	Text string `json:"text"`
}

// wireResp is the (subset of the) ApplyGuardrail response.
type wireResp struct {
	Action      string       `json:"action"` // GUARDRAIL_INTERVENED | NONE
	Outputs     []textBlock  `json:"outputs"`
	Assessments []assessment `json:"assessments"`
}

type assessment struct {
	TopicPolicy                *topicPolicy     `json:"topicPolicy"`
	ContentPolicy              *contentPolicy   `json:"contentPolicy"`
	WordPolicy                 *wordPolicy      `json:"wordPolicy"`
	SensitiveInformationPolicy *sensitivePolicy `json:"sensitiveInformationPolicy"`
}

type topicPolicy struct {
	Topics []struct {
		Name   string `json:"name"`
		Action string `json:"action"`
	} `json:"topics"`
}

type contentPolicy struct {
	Filters []struct {
		Type   string `json:"type"`
		Action string `json:"action"`
	} `json:"filters"`
}

type wordPolicy struct {
	CustomWords []struct {
		Match  string `json:"match"`
		Action string `json:"action"`
	} `json:"customWords"`
}

type sensitivePolicy struct {
	PIIEntities []struct {
		Type   string `json:"type"`
		Action string `json:"action"`
	} `json:"piiEntities"`
}

// Evaluate scans the latest user prompt. Because ApplyGuardrail collapses its
// output, only a single span is sent and the masked result (if any) is written
// back to that span's pointer; the assessments decide block vs mask and produce
// annotations.
func (g *Guardrail) Evaluate(ctx context.Context, req guardrail.Request) (guardrail.Result, error) {
	if len(req.Prompt) == 0 {
		return guardrail.Result{}, nil
	}
	// Bedrock returns a single collapsed output, so we deliberately scan only
	// the latest user prompt (the same span Presidio masks) to keep write-back
	// unambiguous.
	span := req.Prompt[len(req.Prompt)-1]

	body, err := json.Marshal(wireReq{
		Source:  "INPUT",
		Content: []contentItem{{Text: textBlock{Text: span.Value}}},
	})
	if err != nil {
		return guardrail.Result{}, xerrors.Errorf("marshal request: %w", err)
	}

	var out wireResp
	if err := g.send(ctx, body, &out); err != nil {
		return guardrail.Result{}, err
	}

	annotations, blocked := summarize(out.Assessments)
	res := guardrail.Result{Annotations: annotations}

	if strings.ToUpper(out.Action) != "GUARDRAIL_INTERVENED" {
		return res, nil
	}
	if blocked {
		res.Action = guardrail.ActionBlock
		res.Reason = "request blocked by Bedrock guardrail"
		return res, nil
	}
	// Mask: write the collapsed masked output back to the scanned span.
	if len(out.Outputs) > 0 && out.Outputs[0].Text != span.Value {
		res.Edits = []guardrail.Edit{{Pointer: span.Pointer, Value: out.Outputs[0].Text}}
	}
	return res, nil
}

// send signs the request with SigV4 and posts it to the ApplyGuardrail endpoint.
func (g *Guardrail) send(ctx context.Context, body []byte, out any) error {
	url := fmt.Sprintf("%s/guardrail/%s/version/%s/apply",
		strings.TrimRight(g.cfg.Endpoint, "/"), g.cfg.GuardrailIdentifier, g.cfg.GuardrailVersion)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return xerrors.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	sum := sha256.Sum256(body)
	creds := aws.Credentials{
		AccessKeyID:     g.cred.AccessKeyID,
		SecretAccessKey: g.cred.SecretAccessKey,
		SessionToken:    g.cred.SessionToken,
	}
	if err := g.signer.SignHTTP(ctx, creds, httpReq, hex.EncodeToString(sum[:]), service, g.cfg.Region, g.cfg.now()); err != nil {
		return xerrors.Errorf("sign request: %w", err)
	}

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

// summarize folds the assessment block into annotation counts and reports
// whether any non-anonymizing policy (content filter, topic, or word) blocked.
// A sensitive-information policy with ANONYMIZED actions is a mask, not a block.
func summarize(assessments []assessment) (map[string]any, bool) {
	var (
		piiEntities = map[string]int{}
		filters     = map[string]int{}
		topics      []string
		blocked     bool
	)
	for _, a := range assessments {
		if a.SensitiveInformationPolicy != nil {
			for _, e := range a.SensitiveInformationPolicy.PIIEntities {
				piiEntities[e.Type]++
				if strings.EqualFold(e.Action, "BLOCKED") {
					blocked = true
				}
			}
		}
		if a.ContentPolicy != nil {
			for _, f := range a.ContentPolicy.Filters {
				filters[f.Type]++
				if strings.EqualFold(f.Action, "BLOCKED") {
					blocked = true
				}
			}
		}
		if a.TopicPolicy != nil {
			for _, t := range a.TopicPolicy.Topics {
				topics = append(topics, t.Name)
				if strings.EqualFold(t.Action, "BLOCKED") {
					blocked = true
				}
			}
		}
		if a.WordPolicy != nil {
			for _, w := range a.WordPolicy.CustomWords {
				if strings.EqualFold(w.Action, "BLOCKED") {
					blocked = true
				}
			}
		}
	}

	ann := map[string]any{}
	if len(piiEntities) > 0 {
		ann["pii_entities"] = piiEntities
	}
	if len(filters) > 0 {
		ann["content_filters"] = filters
	}
	if len(topics) > 0 {
		ann["topics"] = topics
	}
	ann["blocked"] = blocked
	return ann, blocked
}
