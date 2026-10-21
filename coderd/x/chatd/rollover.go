package chatd

import (
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
)

// contextWindowBudget holds the token budget points of one context
// window under the token budget compaction strategy. A zero
// contextLimit means the window size is unknown and no automatic stage
// fires.
type contextWindowBudget struct {
	contextLimit     int64
	usableLimit      int64
	thresholdPercent int32
}

func newContextWindowBudget(contextLimit, outputCap int64, thresholdPercent int32) contextWindowBudget {
	budget := contextWindowBudget{thresholdPercent: thresholdPercent}
	if contextLimit <= 0 {
		return budget
	}
	budget.contextLimit = contextLimit
	budget.usableLimit = contextLimit - min(outputCap, contextLimit/4)
	return budget
}

type rolloverAction string

const (
	rolloverActionNone     rolloverAction = ""
	rolloverActionRollover rolloverAction = "rollover"
)

type rolloverReason string

const (
	rolloverReasonLimit rolloverReason = "limit"
)

type rolloverDecision struct {
	action rolloverAction
	reason rolloverReason
}

func decideRollover(contextTokens int64, budget contextWindowBudget) rolloverDecision {
	switch {
	case budget.thresholdPercent >= chatloop.CompactionDisabledThresholdPercent:
		return rolloverDecision{action: rolloverActionNone}
	case budget.contextLimit <= 0 || contextTokens <= 0:
		return rolloverDecision{action: rolloverActionNone}
	case contextTokens >= budget.usableLimit:
		return rolloverDecision{action: rolloverActionRollover, reason: rolloverReasonLimit}
	default:
		return rolloverDecision{action: rolloverActionNone}
	}
}
