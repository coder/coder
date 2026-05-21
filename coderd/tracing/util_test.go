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
