package policy_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coder/coder/v2/aibridge/policy"
)

// benchDecidePolicy mirrors the prototype decision policy: it reads
// input.request.body.model and BLOCKs "haiku" models.
const benchDecidePolicy = `
default verdict := "ALLOW"

verdict := "BLOCK" if {
	model := lower(object.get(input.request.body, "model", ""))
	contains(model, "haiku")
}
`

// benchClassifyPolicy attaches a single annotation, to exercise pipeline
// threading cost.
const benchClassifyPolicy = `
annotations := {"is_haiku": contains(lower(object.get(input.request.body, "model", "")), "haiku")}
`

// requestBody builds a body resembling an Anthropic /v1/messages payload with
// msgCount user messages, to measure how cost scales with input (parse) size.
func requestBody(model string, msgCount int) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, `{"model":%q,"messages":[`, model)
	for i := range msgCount {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"role":"user","content":"message number %d with some filler content"}`, i)
	}
	b.WriteString(`]}`)
	return []byte(b.String())
}

var benchBodies = []struct {
	name string
	body []byte
}{
	{"allow/small", requestBody("claude-sonnet-4-5", 1)},
	{"deny/small", requestBody("claude-haiku-4-5", 1)},
	{"allow/large", requestBody("claude-sonnet-4-5", 200)},
	{"deny/large", requestBody("claude-haiku-4-5", 200)},
}

// BenchmarkBuildInput measures the once-per-hook cost of parsing the body into
// the envelope. This dominates for large bodies and is paid a single time
// regardless of how many policies run.
func BenchmarkBuildInput(b *testing.B) {
	for _, tc := range benchBodies {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.body)))
			for b.Loop() {
				if _, err := (policy.PreReqEnvelope{Request: tc.body}).Build(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkDecideEvaluate measures the per-policy marginal cost of evaluating an
// already-built envelope: what each additional decision in a hook costs once
// the body is parsed once by BuildInput.
func BenchmarkDecideEvaluate(b *testing.B) {
	d, err := policy.NewDecide("bench-decide", benchDecidePolicy)
	if err != nil {
		b.Fatal(err)
	}
	ctx := b.Context()
	for _, tc := range benchBodies {
		in, err := policy.PreReqEnvelope{Request: tc.body}.Build()
		if err != nil {
			b.Fatal(err)
		}
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := d.Evaluate(ctx, in); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkPipelineEvaluate measures a classify + decide pipeline over a shared,
// parse-once envelope.
func BenchmarkPipelineEvaluate(b *testing.B) {
	classify, err := policy.NewClassify("bench-classify", benchClassifyPolicy)
	if err != nil {
		b.Fatal(err)
	}
	decide, err := policy.NewDecide("bench-decide", benchDecidePolicy)
	if err != nil {
		b.Fatal(err)
	}
	pipe, err := policy.NewPipeline(policy.PipelineConfig{
		Classify: []*policy.Classify{classify},
		Decide:   []*policy.Decide{decide},
	})
	if err != nil {
		b.Fatal(err)
	}
	ctx := b.Context()
	for _, tc := range benchBodies {
		in, err := policy.PreReqEnvelope{Request: tc.body}.Build()
		if err != nil {
			b.Fatal(err)
		}
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := pipe.Evaluate(ctx, in); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkNewDecide measures policy compilation cost (once at startup, not per
// request).
func BenchmarkNewDecide(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := policy.NewDecide("bench-decide", benchDecidePolicy); err != nil {
			b.Fatal(err)
		}
	}
}
