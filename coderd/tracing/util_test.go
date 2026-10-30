package tracing_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/tracing"
)

// t.Parallel affects the result of these tests.

//nolint:paralleltest
func TestFuncName(t *testing.T) {
	fn := tracing.FuncName()
	assert.Equal(t, "tracing_test.TestFuncName", fn)
}

type foo struct{}

func (foo) bar() string {
	return tracing.FuncName()
}

//nolint:paralleltest
func TestFuncNameMethod(t *testing.T) {
	fn := foo{}.bar()
	assert.Equal(t, "tracing_test.foo.bar", fn)
}

func (*foo) baz() string {
	return tracing.FuncName()
}

//nolint:paralleltest
func TestFuncNameMethodPointer(t *testing.T) {
	fn := (&foo{}).baz()
	assert.Equal(t, "tracing_test.(*foo).baz", fn)
}

func TestRedactURLPath(t *testing.T) {
	t.Parallel()

	for path, want := range map[string]string{
		"/api/experimental/chat-triggers/abc/webhook/secret":       "/api/experimental/chat-triggers/abc/webhook/REDACTED",
		"/api/experimental/chat-triggers/abc/webhook/secret/":      "/api/experimental/chat-triggers/abc/webhook/REDACTED/",
		"/api/experimental/chat-triggers/abc/webhook/secret/extra": "/api/experimental/chat-triggers/abc/webhook/REDACTED/extra",
		"/api/experimental/chat-triggers/abc/other":                "/api/experimental/chat-triggers/abc/other",
		"/api/v2/workspaces/abc":                                   "/api/v2/workspaces/abc",
	} {
		assert.Equal(t, want, tracing.RedactURLPath(path), path)
	}
	assert.Equal(t, "REDACTED", tracing.RedactRouteParam("secret", "value"))
	assert.Equal(t, "value", tracing.RedactRouteParam("trigger", "value"))
}
