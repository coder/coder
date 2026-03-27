//nolint:testpackage // Cache assertions need access to unexported helpers.
package targetexpr

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	evaluator := NewEvaluator()

	tests := []struct {
		name       string
		expression string
		wantErr    string
	}{
		{
			name:       "empty expression",
			expression: "   ",
			wantErr:    "must not be empty",
		},
		{
			name:       "invalid syntax",
			expression: "claims_10m +",
			wantErr:    "invalid target expression",
		},
		{
			name:       "unknown variable",
			expression: "unknown_metric + 1",
			wantErr:    "unknown name unknown_metric",
		},
		{
			name:       "string result",
			expression: `"hello"`,
			wantErr:    "integer target",
		},
		{
			name:       "bool result",
			expression: "claims_10m > 0",
			wantErr:    "integer target",
		},
		{
			name:       "numeric helper expression",
			expression: "min(claims_10m * 10, 20)",
		},
		{
			name:       "scheduled target baseline",
			expression: "max(scheduled_target, 5)",
		},
		{
			name:       "claim and miss totals",
			expression: "claims_5m + misses_5m",
		},
		{
			name:       "clamp helper",
			expression: "clamp(claims_10m * 20, 0, 50)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := evaluator.Validate(tt.expression)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestEvaluate(t *testing.T) {
	t.Parallel()

	evaluator := NewEvaluator()
	env := TargetEnv{
		ScheduledTarget: 3,
		Claims5m:        2,
		Claims10m:       3,
		Claims120m:      2,
		Misses5m:        1,
		ClaimRate5m:     1.5,
	}

	tests := []struct {
		name       string
		expression string
		env        TargetEnv
		want       int32
	}{
		{
			name:       "min helper",
			expression: "min(claims_10m * 10, 20)",
			env:        env,
			want:       20,
		},
		{
			name:       "max baseline",
			expression: "max(scheduled_target, 5)",
			env:        env,
			want:       5,
		},
		{
			name:       "claim and miss sum",
			expression: "claims_5m + misses_5m",
			env:        env,
			want:       3,
		},
		{
			name:       "zero environment",
			expression: "scheduled_target + claims_5m + misses_5m",
			env:        TargetEnv{},
			want:       0,
		},
		{
			name:       "negative output clamps to zero",
			expression: "scheduled_target - 10",
			env:        env,
			want:       0,
		},
		{
			name:       "large output clamps to max",
			expression: "claims_120m * 1000",
			env:        env,
			want:       MaxPrebuildsTarget,
		},
		{
			name:       "ceil result is accepted when integral",
			expression: "ceil(claim_rate_5m * 2)",
			env:        env,
			want:       3,
		},
		{
			name:       "clamp helper",
			expression: "clamp(claims_10m * 20, 0, 50)",
			env:        env,
			want:       50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := evaluator.Evaluate(tt.expression, tt.env)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEvaluateErrors(t *testing.T) {
	t.Parallel()

	evaluator := NewEvaluator()
	env := TargetEnv{ClaimRate5m: 1.25}

	tests := []struct {
		name       string
		expression string
		wantErr    string
	}{
		{
			name:       "division by zero",
			expression: "1 / 0",
			wantErr:    "finite integer target",
		},
		{
			name:       "non integral float result",
			expression: "claim_rate_5m * 2",
			wantErr:    "integer target",
		},
		{
			name:       "invalid helper bounds",
			expression: "clamp(5, 10, 0)",
			wantErr:    "clamp min cannot exceed max",
		},
		{
			name:       "unknown variable",
			expression: "missing_metric + 1",
			wantErr:    "unknown name missing_metric",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := evaluator.Evaluate(tt.expression, env)
			require.Error(t, err)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestCompileCachesPrograms(t *testing.T) {
	t.Parallel()

	evaluator := NewEvaluator()

	firstProgram, err := evaluator.compile("claims_5m + misses_5m")
	require.NoError(t, err)

	secondProgram, err := evaluator.compile("claims_5m + misses_5m")
	require.NoError(t, err)

	require.Same(t, firstProgram, secondProgram)
}

func TestEvaluateIsConcurrentSafe(t *testing.T) {
	t.Parallel()

	evaluator := NewEvaluator()
	expression := "min(claims_10m * 10, 20)"
	env := TargetEnv{Claims10m: 3}

	const goroutines = 32
	const iterationsPerGoroutine = 50

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*iterationsPerGoroutine)

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterationsPerGoroutine {
				got, err := evaluator.Evaluate(expression, env)
				if err != nil {
					errCh <- err
					return
				}
				if got != 20 {
					errCh <- xerrors.New("unexpected evaluation result")
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
}
