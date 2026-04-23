package chatadvisor

// ResultType is the tagged variant of AdvisorResult. Callers should
// compare against the exported constants rather than string literals.
type ResultType string

const (
	// ResultTypeAdvice indicates the advisor returned guidance.
	ResultTypeAdvice ResultType = "advice"
	// ResultTypeLimitReached indicates the per-run advisor budget is exhausted.
	ResultTypeLimitReached ResultType = "limit_reached"
	// ResultTypeError indicates the nested advisor run failed.
	ResultTypeError ResultType = "error"
)

// AdvisorArgs contains the tool-visible advisor question.
type AdvisorArgs struct {
	Question string `json:"question"`
}

// AdvisorResult is the structured result returned by the advisor runtime.
type AdvisorResult struct {
	Type          ResultType `json:"type"`
	Advice        string     `json:"advice,omitempty"`
	Error         string     `json:"error,omitempty"`
	AdvisorModel  string     `json:"advisor_model,omitempty"`
	RemainingUses int        `json:"remaining_uses"`
}
