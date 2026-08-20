package chatopenai

import (
	"fmt"

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

// TransportFor resolves the wire format for a provider and model. override,
// when non-nil, forces the choice instead of consulting the provider SDK's
// known-model list. Azure follows the known-model list because its provider
// exposes no equivalent hook.
func TransportFor(provider, modelID string, override *bool) Transport {
	var useResponses bool
	switch provider {
	case fantasyopenai.Name:
		useResponses = fantasyopenai.IsResponsesModel(modelID)
		if override != nil {
			useResponses = *override
		}
	case fantasyazure.Name:
		useResponses = fantasyopenai.IsResponsesModel(modelID)
	case fantasyopenaicompat.Name:
		// chatd never builds an openai-compat client with Responses enabled.
		return TransportChatCompletions
	default:
		return TransportNotApplicable
	}
	if useResponses {
		return TransportResponses
	}
	return TransportChatCompletions
}
