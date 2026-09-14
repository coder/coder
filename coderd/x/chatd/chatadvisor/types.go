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

type AdvisorArgs struct {
	Question    string  `json:"question" description:"A brief question for the advisor. Must be 2000 runes or fewer. Summarize context instead of pasting long logs or transcripts."`
	ModelIntent *string `json:"model_intent,omitempty" description:"A short, natural-language, present-participle phrase describing why you are consulting the advisor. This is shown to the user as the tool row title, so write it as a standalone action. Use plain English with no underscores or technical jargon. Do not restate the question. Keep it under 100 characters. Good examples: \"Weighing a refactor tradeoff\", \"Checking a migration strategy\", \"Planning a safe rollout\"."`
}

// AdvisorResult is the structured result returned by the advisor runtime.
type AdvisorResult struct {
	Type          ResultType `json:"type"`
	Advice        string     `json:"advice,omitempty"`
	Error         string     `json:"error,omitempty"`
	AdvisorModel  string     `json:"advisor_model,omitempty"`
	RemainingUses int        `json:"remaining_uses"`
}
