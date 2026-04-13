package chattest

import (
	"context"

	"charm.land/fantasy"
)

// FakeModel is a configurable test double for fantasy.LanguageModel.
// Calling a method whose function field is nil panics, forcing tests
// to be explicit about which methods they expect to be invoked.
type FakeModel struct {
	ProviderName     string
	ModelName        string
	GenerateFn       func(context.Context, fantasy.Call) (*fantasy.Response, error)
	StreamFn         func(context.Context, fantasy.Call) (fantasy.StreamResponse, error)
	GenerateObjectFn func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error)
	StreamObjectFn   func(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error)
}

var _ fantasy.LanguageModel = (*FakeModel)(nil)

func (m *FakeModel) Generate(ctx context.Context, call fantasy.Call) (*fantasy.Response, error) {
	if m.GenerateFn == nil {
		panic("chattest: FakeModel.Generate called but GenerateFn is nil")
	}
	return m.GenerateFn(ctx, call)
}

func (m *FakeModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	if m.StreamFn == nil {
		panic("chattest: FakeModel.Stream called but StreamFn is nil")
	}
	return m.StreamFn(ctx, call)
}

func (m *FakeModel) GenerateObject(ctx context.Context, call fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	if m.GenerateObjectFn == nil {
		panic("chattest: FakeModel.GenerateObject called but GenerateObjectFn is nil")
	}
	return m.GenerateObjectFn(ctx, call)
}

func (m *FakeModel) StreamObject(ctx context.Context, call fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	if m.StreamObjectFn == nil {
		panic("chattest: FakeModel.StreamObject called but StreamObjectFn is nil")
	}
	return m.StreamObjectFn(ctx, call)
}

func (m *FakeModel) Provider() string { return m.ProviderName }
func (m *FakeModel) Model() string    { return m.ModelName }
