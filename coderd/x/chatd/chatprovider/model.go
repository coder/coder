package chatprovider

import (
	"charm.land/fantasy"

	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/codersdk"
)

// Model pairs a language model client with the facts resolved when it was
// built. Its fields are unexported and nothing sets transport except NewModel,
// which derives it from the client, so no caller can choose a transport that
// disagrees with the client it wraps.
type Model struct {
	lm        fantasy.LanguageModel
	transport chatopenai.Transport
}

// NewModel pairs an already-built client with the transport resolved from that
// client's own identity. Callers must pass the config the client was built
// with, the one degree of freedom Model cannot police. ModelFromConfig is the
// only production caller.
func NewModel(lm fantasy.LanguageModel, openAIConfig *codersdk.ChatModelOpenAIConfig) Model {
	if lm == nil {
		// The invalid zero value lets callers report a nil client as an
		// error instead of dereferencing it here.
		return Model{}
	}
	return Model{
		lm:        lm,
		transport: chatopenai.TransportFor(lm.Provider(), lm.Model(), openAIResponsesAPIOverride(openAIConfig)),
	}
}

func (m Model) LanguageModel() fantasy.LanguageModel { return m.lm }

func (m Model) Provider() string { return m.lm.Provider() }

func (m Model) ModelID() string { return m.lm.Model() }

func (m Model) Transport() chatopenai.Transport { return m.transport }

// Valid reports whether m wraps a client rather than being the zero value.
func (m Model) Valid() bool { return m.lm != nil }

// WithLanguageModel replaces the wrapped client and keeps the resolved
// transport, because decorators such as debug recording do not change what the
// client speaks. It panics on an invalid receiver or a replacement that
// reports a different identity, because both would pair the kept transport
// with a client it was not resolved from.
func (m Model) WithLanguageModel(lm fantasy.LanguageModel) Model {
	if !m.Valid() {
		panic("chatprovider: WithLanguageModel on an invalid Model")
	}
	if lm == nil || lm.Provider() != m.lm.Provider() || lm.Model() != m.lm.Model() {
		panic("chatprovider: WithLanguageModel replacement changes the model identity")
	}
	m.lm = lm
	return m
}
