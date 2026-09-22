package chatopenai

import (
	"fmt"
	"strings"

	fantasyazure "charm.land/fantasy/providers/azure"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
)

// Transport identifies the OpenAI wire format a model's client speaks. It is
// resolved once when the client is built so that request preparation cannot
// disagree with the client it prepares for.
type Transport int

const (
	// TransportInvalid is the zero value, meaning no transport was ever
	// resolved.
	TransportInvalid Transport = iota
	// TransportNotApplicable marks providers outside the OpenAI wire formats.
	TransportNotApplicable
	TransportChatCompletions
	TransportResponses
)

// UsesResponses reports whether t selects the Responses API. It panics on
// TransportInvalid instead of defaulting, because an unresolved transport is a
// construction bug rather than user input.
func (t Transport) UsesResponses() bool {
	switch t {
	case TransportResponses:
		return true
	case TransportChatCompletions, TransportNotApplicable:
		return false
	default:
		panic(fmt.Sprintf("chatopenai: unresolved transport %d", int(t)))
	}
}

// TransportFor resolves the wire format for a provider and model. OpenAI
// models use override when set and otherwise consult the provider SDK's
// known-model list. OpenAI-compatible models use Chat Completions unless an
// explicit override selects Responses, preserving their generic default.
func TransportFor(provider, modelID string, override *bool) Transport {
	var useResponses bool
	switch provider {
	case fantasyopenai.Name:
		useResponses = UsesResponsesAPI(modelID, override)
	case fantasyazure.Name:
		useResponses = fantasyopenai.IsResponsesModel(modelID)
	case fantasyopenaicompat.Name:
		useResponses = override != nil && *override
	default:
		return TransportNotApplicable
	}
	if useResponses {
		return TransportResponses
	}
	return TransportChatCompletions
}

// UsesResponsesAPI decides the wire format for an OpenAI model. override, when
// non-nil, wins. Otherwise the provider SDK's known-model list decides.
func UsesResponsesAPI(modelID string, override *bool) bool {
	if override != nil {
		return *override
	}
	return fantasyopenai.IsResponsesModel(modelID)
}

// IsGPT6Astra matches gpt-6-astra and its dated snapshots.
func IsGPT6Astra(modelID string) bool {
	return strings.HasPrefix(strings.ToLower(modelID), "gpt-6-astra")
}
