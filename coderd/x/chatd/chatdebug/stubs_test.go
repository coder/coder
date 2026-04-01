package chatdebug //nolint:testpackage // Branch-02 test shims need package-private names.

import (
	"context"
	"net/http"

	"charm.land/fantasy"
)

// This branch-02 test compatibility shim forward-declares later branch
// symbols needed for test compilation. Delete it once recorder.go,
// transport.go, and redaction.go are available here.

type stubModel struct {
	provider string
	model    string
}

func (*stubModel) Generate(
	context.Context,
	fantasy.Call,
) (*fantasy.Response, error) {
	return &fantasy.Response{}, nil
}

func (*stubModel) Stream(
	context.Context,
	fantasy.Call,
) (fantasy.StreamResponse, error) {
	return fantasy.StreamResponse(func(func(fantasy.StreamPart) bool) {}), nil
}

func (*stubModel) GenerateObject(
	context.Context,
	fantasy.ObjectCall,
) (*fantasy.ObjectResponse, error) {
	return &fantasy.ObjectResponse{}, nil
}

func (*stubModel) StreamObject(
	context.Context,
	fantasy.ObjectCall,
) (fantasy.ObjectStreamResponse, error) {
	return fantasy.ObjectStreamResponse(func(func(fantasy.ObjectStreamPart) bool) {}), nil
}

func (s *stubModel) Provider() string {
	return s.provider
}

func (s *stubModel) Model() string {
	return s.model
}

// RedactedValue replaces sensitive values in debug payloads.
const RedactedValue = "[REDACTED]"

// RecordingTransport is the branch-02 placeholder HTTP recording transport.
type RecordingTransport struct {
	Base http.RoundTripper
}

var _ http.RoundTripper = (*RecordingTransport)(nil)

func (t *RecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		panic("chatdebug: nil request")
	}

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
