package experiments

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"unicode/utf8"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/ext"
	"cel.dev/cel-go/interpreter"
	"golang.org/x/xerrors"
)

const (
	// MaxConditionSize is the maximum condition length in code points.
	MaxConditionSize = 4096
	// maxConditionCost bounds the runtime cost of one evaluation.
	maxConditionCost = 10_000
	// interruptCheckFrequency is how many comprehension iterations run
	// between context cancellation checks.
	interruptCheckFrequency = 100
	// maxCachedPrograms bounds the compiled program cache.
	maxCachedPrograms = 256
)

// errorCategory names a failure class. Categories are the only error
// detail written to server logs.
type errorCategory string

const (
	categorySyntax    errorCategory = "syntax"
	categoryType      errorCategory = "type"
	categorySize      errorCategory = "size"
	categoryCost      errorCategory = "cost"
	categoryEval      errorCategory = "eval"
	categoryCancelled errorCategory = "canceled"
	categoryMalformed errorCategory = "malformed"
	categoryRead      errorCategory = "read"
)

// conditionError is a compile or evaluation failure. Error returns the
// full CEL diagnostics, which can quote the condition, so loggers must use
// only category, line and column.
type conditionError struct {
	category errorCategory
	// line is 1-based and column is 1-based. Zero means unknown.
	line   int
	column int
	diag   error
}

func (e *conditionError) Error() string { return e.diag.Error() }

func (e *conditionError) Unwrap() error { return e.diag }

// celEnv is shared by every compile. A *cel.Env is safe for concurrent
// use.
var celEnv = sync.OnceValues(func() (*cel.Env, error) {
	// The user variable's type name is "<package>.<type>", as named by
	// ext.NativeTypes.
	return cel.NewEnv(
		ext.NativeTypes(reflect.TypeFor[User](), ext.ParseStructTags(true)),
		cel.Variable("user", cel.ObjectType("experiments.User")),
		cel.ParserExpressionSizeLimit(MaxConditionSize),
	)
})

// compileCondition parses, type-checks and plans source. It requires a
// bool result.
func compileCondition(source string) (cel.Program, error) {
	if utf8.RuneCountInString(source) > MaxConditionSize {
		return nil, &conditionError{
			category: categorySize,
			diag:     xerrors.Errorf("condition exceeds %d characters", MaxConditionSize),
		}
	}
	env, err := celEnv()
	if err != nil {
		return nil, xerrors.Errorf("create CEL environment: %w", err)
	}
	parsed, iss := env.Parse(source)
	if iss.Err() != nil {
		return nil, issuesError(categorySyntax, iss)
	}
	checked, iss := env.Check(parsed)
	if iss.Err() != nil {
		return nil, issuesError(categoryType, iss)
	}
	if !checked.OutputType().IsExactType(cel.BoolType) {
		return nil, &conditionError{
			category: categoryType,
			diag:     xerrors.Errorf("condition must return bool, got %s", checked.OutputType()),
		}
	}
	prg, err := env.Program(checked,
		cel.CostLimit(maxConditionCost),
		cel.InterruptCheckFrequency(interruptCheckFrequency),
	)
	if err != nil {
		return nil, &conditionError{category: categoryType, diag: xerrors.Errorf("plan condition: %w", err)}
	}
	return prg, nil
}

func issuesError(category errorCategory, iss *cel.Issues) *conditionError {
	cerr := &conditionError{category: category, diag: iss.Err()}
	if errs := iss.Errors(); len(errs) > 0 && errs[0].Location != nil {
		cerr.line = errs[0].Location.Line()
		cerr.column = errs[0].Location.Column() + 1
	}
	return cerr
}

// evalCondition runs prg for user. The context cancels evaluation.
func evalCondition(ctx context.Context, prg cel.Program, user User) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, &conditionError{category: categoryCancelled, diag: err}
	}
	out, _, err := prg.ContextEval(ctx, map[string]any{"user": user})
	if err != nil {
		var canceled interpreter.EvalCancelledError
		switch {
		case errors.As(err, &canceled) && canceled.Cause == interpreter.CostLimitExceeded:
			return false, &conditionError{category: categoryCost, diag: err}
		case errors.As(err, &canceled), ctx.Err() != nil:
			return false, &conditionError{category: categoryCancelled, diag: err}
		default:
			return false, &conditionError{category: categoryEval, diag: err}
		}
	}
	result, ok := out.Value().(bool)
	if !ok {
		return false, &conditionError{category: categoryEval, diag: xerrors.Errorf("condition returned %s, not bool", out.Type())}
	}
	return result, nil
}

// compiled is one cache entry. Compile failures are cached too, so a
// broken stored condition is not recompiled on every call.
type compiled struct {
	prg cel.Program
	err error
}

// programCache holds compiled conditions keyed by exact source text. It
// is bounded and safe for concurrent use.
type programCache struct {
	mu       sync.Mutex
	programs map[string]compiled
}

func newProgramCache() *programCache {
	return &programCache{programs: make(map[string]compiled)}
}

func (c *programCache) get(source string) (cel.Program, error) {
	c.mu.Lock()
	entry, ok := c.programs[source]
	c.mu.Unlock()
	if ok {
		return entry.prg, entry.err
	}

	// Compile outside the lock. Concurrent misses for the same source
	// may compile twice; the results are equivalent.
	prg, err := compileCondition(source)

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.programs[source]; !ok && len(c.programs) >= maxCachedPrograms {
		// Evict an arbitrary entry to stay within the bound.
		for k := range c.programs {
			delete(c.programs, k)
			break
		}
	}
	c.programs[source] = compiled{prg: prg, err: err}
	return prg, err
}
