// Package targetexpr evaluates prebuild target count expressions.
package targetexpr

import (
	"math"
	"reflect"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"golang.org/x/xerrors"
)

const (
	// MaxPrebuildsTarget is the hard safety ceiling for evaluated targets.
	MaxPrebuildsTarget int32 = 100
)

// Evaluator validates and evaluates prebuild scaling expressions.
type Evaluator interface {
	// Validate checks expression syntax and that it only references known variables.
	// Returns a user-friendly error if invalid.
	Validate(expression string) error

	// Evaluate runs the expression with the given environment and returns the
	// desired prebuild count. Returns an error if evaluation fails at runtime.
	Evaluate(expression string, env TargetEnv) (int32, error)
}

// TargetEnv holds the variables available to scaling expressions.
type TargetEnv struct {
	// ScheduledTarget is the baseline from static desired_instances or cron schedule.
	ScheduledTarget int32 `expr:"scheduled_target"`

	// Current state counts.
	Running  int32 `expr:"running"`
	Eligible int32 `expr:"eligible"`
	Starting int32 `expr:"starting"`
	Stopping int32 `expr:"stopping"`
	Deleting int32 `expr:"deleting"`
	Expired  int32 `expr:"expired"`

	// Rolling-window claim counts.
	Claims5m   int32 `expr:"claims_5m"`
	Claims10m  int32 `expr:"claims_10m"`
	Claims30m  int32 `expr:"claims_30m"`
	Claims60m  int32 `expr:"claims_60m"`
	Claims120m int32 `expr:"claims_120m"`

	// Rolling-window miss counts.
	Misses5m   int32 `expr:"misses_5m"`
	Misses10m  int32 `expr:"misses_10m"`
	Misses30m  int32 `expr:"misses_30m"`
	Misses60m  int32 `expr:"misses_60m"`
	Misses120m int32 `expr:"misses_120m"`

	// Rolling-window claim rates (per minute).
	ClaimRate5m   float64 `expr:"claim_rate_5m"`
	ClaimRate10m  float64 `expr:"claim_rate_10m"`
	ClaimRate30m  float64 `expr:"claim_rate_30m"`
	ClaimRate60m  float64 `expr:"claim_rate_60m"`
	ClaimRate120m float64 `expr:"claim_rate_120m"`

	// Time-of-day fields (derived from preset timezone).
	Hour    int `expr:"hour"`
	Weekday int `expr:"weekday"`
}

// ExprEvaluator validates and evaluates target expressions with expr-lang/expr.
type ExprEvaluator struct {
	// programs caches compiled programs by normalized expression string.
	// sync.Map keeps repeated validation and evaluation safe under concurrent use.
	programs sync.Map // map[string]*vm.Program
}

var _ Evaluator = (*ExprEvaluator)(nil)

var exprCompileOptions = []expr.Option{
	expr.Env(TargetEnv{}),
	expr.AsAny(),
	expr.Function("clamp", clampExpr, new(func(int, int, int) int)),
}

// NewEvaluator returns an Expr-backed evaluator.
func NewEvaluator() *ExprEvaluator {
	return &ExprEvaluator{}
}

// Validate checks that an expression is syntactically valid, only references
// known variables, and produces a numeric result.
func (e *ExprEvaluator) Validate(expression string) error {
	_, err := e.compile(expression)
	return err
}

// Evaluate runs an expression against env and returns the clamped target.
func (e *ExprEvaluator) Evaluate(expression string, env TargetEnv) (int32, error) {
	program, err := e.compile(expression)
	if err != nil {
		return 0, err
	}

	result, err := expr.Run(program, env)
	if err != nil {
		return 0, xerrors.Errorf("failed to evaluate target expression: %w", err)
	}

	target, err := normalizeTarget(result)
	if err != nil {
		return 0, err
	}

	return clampTarget(target), nil
}

func (e *ExprEvaluator) compile(expression string) (*vm.Program, error) {
	normalizedExpression := strings.TrimSpace(expression)
	if normalizedExpression == "" {
		return nil, xerrors.New("expression must not be empty")
	}

	if cachedProgram, ok := e.loadProgram(normalizedExpression); ok {
		return cachedProgram, nil
	}

	program, err := expr.Compile(normalizedExpression, exprCompileOptions...)
	if err != nil {
		return nil, xerrors.Errorf("invalid target expression: %w", err)
	}
	if program == nil {
		return nil, xerrors.New("failed to compile target expression: compiled program is nil")
	}

	if err := validateProgramOutput(program); err != nil {
		return nil, err
	}

	// Compile once with AsInt after the stricter AsAny validation passes. This
	// keeps the implementation aligned with expr's integer-target option without
	// relying on its runtime truncation semantics.
	if _, err := expr.Compile(normalizedExpression, expr.Env(TargetEnv{}), expr.AsInt(), expr.Function("clamp", clampExpr, new(func(int, int, int) int))); err != nil {
		return nil, xerrors.Errorf("invalid target expression: %w", err)
	}
	actualProgram, loaded := e.programs.LoadOrStore(normalizedExpression, program)
	if !loaded {
		return program, nil
	}

	cachedProgram, ok := actualProgram.(*vm.Program)
	if !ok || cachedProgram == nil {
		return nil, xerrors.New("failed to load cached target expression program")
	}

	return cachedProgram, nil
}

func (e *ExprEvaluator) loadProgram(expression string) (*vm.Program, bool) {
	cachedProgram, ok := e.programs.Load(expression)
	if !ok {
		return nil, false
	}

	program, ok := cachedProgram.(*vm.Program)
	if !ok || program == nil {
		return nil, false
	}

	return program, true
}

func validateProgramOutput(program *vm.Program) error {
	outputType := program.Node().Type()
	if outputType == nil {
		return xerrors.New("invalid target expression: expression output type is unknown")
	}

	if !isNumericKind(outputType.Kind()) {
		return xerrors.Errorf(
			"invalid target expression: expression must evaluate to an integer target, got %s",
			outputType,
		)
	}

	return nil
}

func isNumericKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func normalizeTarget(result any) (int64, error) {
	switch value := result.(type) {
	case int:
		return int64(value), nil
	case int8:
		return int64(value), nil
	case int16:
		return int64(value), nil
	case int32:
		return int64(value), nil
	case int64:
		return value, nil
	case uint:
		if value > uint(math.MaxInt64) {
			return 0, xerrors.Errorf("expression result %v is out of range", value)
		}
		return int64(value), nil
	case uint8:
		return int64(value), nil
	case uint16:
		return int64(value), nil
	case uint32:
		return int64(value), nil
	case uint64:
		if value > math.MaxInt64 {
			return 0, xerrors.Errorf("expression result %v is out of range", value)
		}
		return int64(value), nil
	case uintptr:
		if uint64(value) > math.MaxInt64 {
			return 0, xerrors.Errorf("expression result %v is out of range", value)
		}
		return int64(value), nil
	case float32:
		return normalizeFloat(float64(value))
	case float64:
		return normalizeFloat(value)
	default:
		return 0, xerrors.Errorf(
			"expression must evaluate to an integer target, got %T",
			result,
		)
	}
}

func normalizeFloat(value float64) (int64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, xerrors.Errorf(
			"expression must evaluate to a finite integer target, got %v",
			value,
		)
	}
	if math.Trunc(value) != value {
		return 0, xerrors.Errorf(
			"expression must evaluate to an integer target, got %v",
			value,
		)
	}
	if value < math.MinInt64 || value > math.MaxInt64 {
		return 0, xerrors.Errorf("expression result %v is out of range", value)
	}

	return int64(value), nil
}

func clampTarget(value int64) int32 {
	if value <= 0 {
		return 0
	}
	if value >= int64(MaxPrebuildsTarget) {
		return MaxPrebuildsTarget
	}
	return int32(value)
}

// clampExpr backs the expr clamp() helper. The explicit checks here document
// the expected argument contract in case the function is ever reused outside of
// expr's compile-time type checker.
func clampExpr(params ...any) (any, error) {
	if len(params) != 3 {
		return nil, xerrors.Errorf("clamp expects 3 arguments, got %d", len(params))
	}

	value, ok := params[0].(int)
	if !ok {
		return nil, xerrors.Errorf("clamp value must be an int, got %T", params[0])
	}
	minValue, ok := params[1].(int)
	if !ok {
		return nil, xerrors.Errorf("clamp min must be an int, got %T", params[1])
	}
	maxValue, ok := params[2].(int)
	if !ok {
		return nil, xerrors.Errorf("clamp max must be an int, got %T", params[2])
	}
	if minValue > maxValue {
		return nil, xerrors.Errorf("clamp min cannot exceed max")
	}

	return clamp(value, minValue, maxValue), nil
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
