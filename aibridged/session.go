package aibridged

import (
	"fmt"
	"sync/atomic"
)

type OpenAIToolCall struct {
	funcName string
	args     map[string]string
}

type OpenAIToolCallState int

const (
	OpenAIToolCallNotReady OpenAIToolCallState = iota
	OpenAIToolCallReady
	OpenAIToolCallInProgress
	OpenAIToolCallDone
)

func (o OpenAIToolCallState) String() string {
	switch o {
	case OpenAIToolCallNotReady:
		return "not ready"
	case OpenAIToolCallReady:
		return "ready"
	case OpenAIToolCallInProgress:
		return "in-progress"
	case OpenAIToolCallDone:
		return "done"
	default:
		return fmt.Sprintf("UNKNOWN STATE: %d", o)
	}
}

type OpenAISession struct {
	done atomic.Bool
	// key = tool call ID
	toolCallsRequired map[string]*OpenAIToolCall
	toolCallsState    map[string]OpenAIToolCallState
	phantomEvents     [][]byte
}

func NewOpenAISession() *OpenAISession {
	return &OpenAISession{
		toolCallsRequired: make(map[string]*OpenAIToolCall),
		toolCallsState:    make(map[string]OpenAIToolCallState),
	}
}
