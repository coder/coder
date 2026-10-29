package chatopenai

import (
	"fmt"
	"strings"

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

// TransportFor resolves the wire format for a provider and model. For OpenAI,
// a non-nil override takes precedence over the provider SDK's known-model list.
func TransportFor(provider, modelID string, override *bool) Transport {
	switch provider {
	case fantasyopenai.Name:
		if UsesResponsesAPI(modelID, override) {
			return TransportResponses
		}
		return TransportChatCompletions
	case fantasyopenaicompat.Name:
		// chatd never builds an openai-compat client with Responses enabled.
		return TransportChatCompletions
	default:
		return TransportNotApplicable
	}
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

// IsGPT61Sol matches gpt-6.1-sol and its dated snapshots.
func IsGPT61Sol(modelID string) bool {
	return strings.HasPrefix(strings.ToLower(modelID), "gpt-6.1-sol")
}
