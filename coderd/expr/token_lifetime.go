package expr

import (
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"golang.org/x/xerrors"
)

// Subject is a simplified version of rbac.Subject for expr evaluation
type Subject struct {
	ID     string
	Email  string
	Groups []string
	Roles  []Role
}

// Role represents a role with optional organization scope
type Role struct {
	Name  string
	OrgID string // Empty string for site-wide roles
}

// CompileTokenLifetimeExpression compiles an expr expression for token lifetime evaluation.
// The expression must return a time.Duration value.
//
// Note: We use AsInt64() because time.Duration's underlying type is int64, and expr's
// type system works with basic Go types. The duration() function and environment
// variables return int64 values that represent nanoseconds, which can be safely
// cast to time.Duration.
func CompileTokenLifetimeExpression(expression string) (*vm.Program, error) {
	env := []expr.Option{
		// Define the environment with expected types
		expr.Env(map[string]any{
			"subject":           Subject{},
			"globalMaxDuration": int64(0), // time.Duration as int64
			"defaultDuration":   int64(0), // time.Duration as int64
		}),
		// Define custom functions
		expr.Function(
			"duration",
			func(params ...any) (any, error) {
				if len(params) != 1 {
					return nil, xerrors.New("duration() expects exactly 1 argument")
				}

				switch arg := params[0].(type) {
				case string:
					d, err := time.ParseDuration(arg)
					if err != nil {
						return nil, err
					}

					// Return as int64 to match AsInt64() constraint
					return int64(d), nil
				case int64:
					return arg, nil
				default:
					return nil, xerrors.New("unsupported parameter type")
				}
			},
			new(func(string) (int64, error)),
			new(func(int64) (int64, error)),
		),
		// Ensure the expression returns an int64 (time.Duration's underlying type)
		// This provides compile-time type safety
		expr.AsInt64(),
	}

	return expr.Compile(expression, env...)
}

// TODO: Add timeout protection for expr evaluation in future iterations
// Example: EvaluateWithTimeout(ctx, program, vars, 100*time.Millisecond)

// TODO: Add expression complexity limits in future iterations
// Example: Check AST node count or expression depth
