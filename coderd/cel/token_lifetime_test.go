package cel_test

import (
	"testing"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cel2 "github.com/coder/coder/v2/coderd/cel"
	"github.com/coder/coder/v2/coderd/rbac"
)

func TestConvertSubjectToCEL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		subject  rbac.Subject
		expected cel2.Subject
	}{
		{
			name: "BasicUserWithSiteWideRoles",
			subject: rbac.Subject{
				ID:     "user-123",
				Email:  "user@example.com",
				Groups: []string{"developers", "admins"},
				Roles: rbac.RoleIdentifiers{
					rbac.RoleIdentifier{Name: "owner", OrganizationID: uuid.Nil},
					rbac.RoleIdentifier{Name: "member", OrganizationID: uuid.Nil},
				},
			},
			expected: cel2.Subject{
				ID:     "user-123",
				Email:  "user@example.com",
				Groups: []string{"developers", "admins"},
				Roles: []cel2.Role{
					{Name: "owner", OrgID: ""},
					{Name: "member", OrgID: ""},
				},
			},
		},
		{
			name: "UserWithOrganizationRoles",
			subject: rbac.Subject{
				ID:     "user-456",
				Email:  "user@company.com",
				Groups: []string{},
				Roles: rbac.RoleIdentifiers{
					rbac.RoleIdentifier{Name: "organization-admin", OrganizationID: uuid.MustParse("12345678-1234-1234-1234-123456789abc")},
					rbac.RoleIdentifier{Name: "organization-member", OrganizationID: uuid.MustParse("87654321-4321-4321-4321-cba987654321")},
				},
			},
			expected: cel2.Subject{
				ID:     "user-456",
				Email:  "user@company.com",
				Groups: []string{},
				Roles: []cel2.Role{
					{Name: "organization-admin", OrgID: "12345678-1234-1234-1234-123456789abc"},
					{Name: "organization-member", OrgID: "87654321-4321-4321-4321-cba987654321"},
				},
			},
		},
		{
			name: "UserWithMixedRoles",
			subject: rbac.Subject{
				ID:     "user-789",
				Email:  "admin@example.com",
				Groups: []string{"leadership"},
				Roles: rbac.RoleIdentifiers{
					rbac.RoleIdentifier{Name: "owner", OrganizationID: uuid.Nil},
					rbac.RoleIdentifier{Name: "organization-admin", OrganizationID: uuid.MustParse("11111111-2222-3333-4444-555555555555")},
				},
			},
			expected: cel2.Subject{
				ID:     "user-789",
				Email:  "admin@example.com",
				Groups: []string{"leadership"},
				Roles: []cel2.Role{
					{Name: "owner", OrgID: ""},
					{Name: "organization-admin", OrgID: "11111111-2222-3333-4444-555555555555"},
				},
			},
		},
		{
			name: "EmptyUser",
			subject: rbac.Subject{
				ID:     "",
				Email:  "",
				Groups: []string{},
				Roles:  rbac.RoleIdentifiers{},
			},
			expected: cel2.Subject{
				ID:     "",
				Email:  "",
				Groups: []string{},
				Roles:  []cel2.Role{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := cel2.ConvertSubjectToCEL(tt.subject)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewTokenLifetimeEnvironment(t *testing.T) {
	t.Parallel()

	t.Run("BasicEnvironment", func(t *testing.T) {
		t.Parallel()
		env, err := cel2.NewTokenLifetimeEnvironment(cel2.EnvironmentOptions{})
		require.NoError(t, err)
		require.NotNil(t, env)

		// Test compilation of a basic expression
		ast, issues := env.Compile("subject.id")
		require.NoError(t, issues.Err())
		require.NotNil(t, ast)
	})

	t.Run("EnvironmentWithDurationFunction", func(t *testing.T) {
		t.Parallel()
		env, err := cel2.NewTokenLifetimeEnvironment(cel2.EnvironmentOptions{})
		require.NoError(t, err)
		require.NotNil(t, env)

		// Test compilation of an expression using the duration function
		ast, issues := env.Compile(`duration("1h")`)
		require.NoError(t, issues.Err())
		require.NotNil(t, ast)

		// Test that the duration function works
		program, err := env.Program(ast)
		require.NoError(t, err)

		out, _, err := program.Eval(map[string]interface{}{})
		require.NoError(t, err)

		// Check that the result is a duration (could be types.Duration or time.Duration)
		switch v := out.Value().(type) {
		case types.Duration:
			assert.Equal(t, time.Hour, v.Duration)
		case time.Duration:
			assert.Equal(t, time.Hour, v)
		default:
			t.Fatalf("Expected duration type, got %T", out.Value())
		}
	})

	t.Run("RoleOwnerCheck", func(t *testing.T) {
		t.Parallel()
		env, err := cel2.NewTokenLifetimeEnvironment(cel2.EnvironmentOptions{})
		require.NoError(t, err)
		require.NotNil(t, env)

		// Test compilation of expression that checks for owner role
		ast, issues := env.Compile(`subject.roles.exists(r, r.name == "owner")`)
		require.NoError(t, issues.Err())
		require.NotNil(t, ast)

		program, err := env.Program(ast)
		require.NoError(t, err)

		// Test with a subject that has owner role
		subjectWithOwner := cel2.Subject{
			ID:    "user-123",
			Email: "owner@example.com",
			Roles: []cel2.Role{
				{Name: "owner", OrgID: ""},
				{Name: "member", OrgID: ""},
			},
		}

		out, _, err := program.Eval(map[string]interface{}{
			"subject": subjectWithOwner,
		})
		require.NoError(t, err)
		assert.Equal(t, true, out.Value())

		// Test with a subject that doesn't have owner role
		subjectWithoutOwner := cel2.Subject{
			ID:    "user-456",
			Email: "member@example.com",
			Roles: []cel2.Role{
				{Name: "member", OrgID: ""},
				{Name: "template-admin", OrgID: ""},
			},
		}

		out, _, err = program.Eval(map[string]interface{}{
			"subject": subjectWithoutOwner,
		})
		require.NoError(t, err)
		assert.Equal(t, false, out.Value())
	})

	t.Run("DurationArithmetic", func(t *testing.T) {
		t.Parallel()
		env, err := cel2.NewTokenLifetimeEnvironment(cel2.EnvironmentOptions{})
		require.NoError(t, err)
		require.NotNil(t, env)

		// Test compilation of expression that subtracts globalMaxDuration from defaultDuration
		ast, issues := env.Compile(`defaultDuration - globalMaxDuration`)
		require.NoError(t, issues.Err())
		require.NotNil(t, ast)

		program, err := env.Program(ast)
		require.NoError(t, err)

		// Test with sample durations
		defaultDur := 24 * time.Hour
		globalMaxDur := 8 * time.Hour
		expected := defaultDur - globalMaxDur

		out, _, err := program.Eval(map[string]interface{}{
			"defaultDuration":   defaultDur,
			"globalMaxDuration": globalMaxDur,
		})
		require.NoError(t, err)

		// Check that the result is a duration
		switch v := out.Value().(type) {
		case types.Duration:
			assert.Equal(t, expected, v.Duration)
		case time.Duration:
			assert.Equal(t, expected, v)
		default:
			t.Fatalf("Expected duration type, got %T", out.Value())
		}
	})

	t.Run("ConditionalTokenLifetime", func(t *testing.T) {
		t.Parallel()
		env, err := cel2.NewTokenLifetimeEnvironment(cel2.EnvironmentOptions{})
		require.NoError(t, err)
		require.NotNil(t, env)

		// Test compilation of expression that sets different lifetimes based on role
		ast, issues := env.Compile(`subject.roles.exists(r, r.name == "owner") ? duration("720h") : duration("24h")`)
		require.NoError(t, issues.Err())
		require.NotNil(t, ast)

		program, err := env.Program(ast)
		require.NoError(t, err)

		// Test with owner role (should get 720h = 30 days)
		ownerSubject := cel2.Subject{
			ID:    "owner-123",
			Email: "owner@example.com",
			Roles: []cel2.Role{
				{Name: "owner", OrgID: ""},
			},
		}

		out, _, err := program.Eval(map[string]interface{}{
			"subject": ownerSubject,
		})
		require.NoError(t, err)

		switch v := out.Value().(type) {
		case types.Duration:
			assert.Equal(t, 720*time.Hour, v.Duration)
		case time.Duration:
			assert.Equal(t, 720*time.Hour, v)
		default:
			t.Fatalf("Expected duration type, got %T", out.Value())
		}

		// Test with non-owner role (should get 24h)
		memberSubject := cel2.Subject{
			ID:    "member-456",
			Email: "member@example.com",
			Roles: []cel2.Role{
				{Name: "member", OrgID: ""},
			},
		}

		out, _, err = program.Eval(map[string]interface{}{
			"subject": memberSubject,
		})
		require.NoError(t, err)

		switch v := out.Value().(type) {
		case types.Duration:
			assert.Equal(t, 24*time.Hour, v.Duration)
		case time.Duration:
			assert.Equal(t, 24*time.Hour, v)
		default:
			t.Fatalf("Expected duration type, got %T", out.Value())
		}
	})
}

func TestDurationFunction(t *testing.T) {
	t.Parallel()

	// Create an environment with just the duration function
	env, err := cel2.NewTokenLifetimeEnvironment(cel2.EnvironmentOptions{})
	require.NoError(t, err)

	tests := []struct {
		name        string
		expression  string
		expected    time.Duration
		expectError bool
	}{
		{
			name:       "ValidHours",
			expression: `duration("24h")`,
			expected:   24 * time.Hour,
		},
		{
			name:       "ValidMinutes",
			expression: `duration("30m")`,
			expected:   30 * time.Minute,
		},
		{
			name:       "ValidSeconds",
			expression: `duration("45s")`,
			expected:   45 * time.Second,
		},
		{
			name:       "ValidComplex",
			expression: `duration("1h30m45s")`,
			expected:   time.Hour + 30*time.Minute + 45*time.Second,
		},
		{
			name:        "InvalidFormat",
			expression:  `duration("invalid")`,
			expectError: true,
		},
		{
			name:        "EmptyString",
			expression:  `duration("")`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ast, issues := env.Compile(tt.expression)
			require.NoError(t, issues.Err())

			program, err := env.Program(ast)
			require.NoError(t, err)

			out, _, err := program.Eval(map[string]interface{}{})

			if tt.expectError {
				// For invalid durations, CEL should return an error value
				assert.True(t, types.IsError(out))
			} else {
				require.NoError(t, err)
				// Check that the result is a duration (could be types.Duration or time.Duration)
				switch v := out.Value().(type) {
				case types.Duration:
					assert.Equal(t, tt.expected, v.Duration)
				case time.Duration:
					assert.Equal(t, tt.expected, v)
				default:
					t.Fatalf("Expected duration type, got %T", out.Value())
				}
			}
		})
	}
}

func TestEnvironmentOptions(t *testing.T) {
	t.Parallel()

	t.Run("WithExtraOptions", func(t *testing.T) {
		t.Parallel()
		// Test that extra options are properly included
		extraVar := "extraVariable"
		opts := cel2.EnvironmentOptions{
			ExtraOptions: []cel.EnvOption{
				cel.Variable(extraVar, cel.StringType),
			},
		}

		env, err := cel2.NewTokenLifetimeEnvironment(opts)
		require.NoError(t, err)

		// Test that we can use the extra variable
		ast, issues := env.Compile(extraVar)
		require.NoError(t, issues.Err())

		program, err := env.Program(ast)
		require.NoError(t, err)

		out, _, err := program.Eval(map[string]interface{}{
			extraVar: "test value",
		})
		require.NoError(t, err)
		assert.Equal(t, "test value", out.Value())
	})
}

// Benchmarks for CEL expression evaluation performance
func BenchmarkTokenLifetimeEvaluation(b *testing.B) {
	// Create environment once
	env, err := cel2.NewTokenLifetimeEnvironment(cel2.EnvironmentOptions{})
	require.NoError(b, err)

	// Common test subject
	subject := cel2.Subject{
		ID:     "user-123",
		Email:  "user@example.com",
		Groups: []string{"developers", "admins"},
		Roles: []cel2.Role{
			{Name: "owner", OrgID: ""},
			{Name: "member", OrgID: ""},
			{Name: "organization-admin", OrgID: "12345678-1234-1234-1234-123456789abc"},
		},
	}

	benchmarks := []struct {
		name       string
		expression string
		setupVars  map[string]interface{}
	}{
		{
			name:       "SimpleRoleCheck",
			expression: `subject.roles.exists(r, r.name == "owner")`,
			setupVars: map[string]interface{}{
				"subject": subject,
			},
		},
		{
			name:       "ConditionalDuration",
			expression: `subject.roles.exists(r, r.name == "owner") ? duration("720h") : duration("24h")`,
			setupVars: map[string]interface{}{
				"subject": subject,
			},
		},
		{
			name:       "ComplexRoleLogic",
			expression: `subject.roles.exists(r, r.name == "owner" && r.orgID == "") ? duration("720h") : (subject.roles.exists(r, r.name == "organization-admin") ? duration("168h") : duration("24h"))`,
			setupVars: map[string]interface{}{
				"subject": subject,
			},
		},
		{
			name:       "GroupMembership",
			expression: `"developers" in subject.groups ? duration("168h") : duration("24h")`,
			setupVars: map[string]interface{}{
				"subject": subject,
			},
		},
		{
			name:       "EmailDomainCheck",
			expression: `subject.email.endsWith("@example.com") ? duration("336h") : duration("24h")`,
			setupVars: map[string]interface{}{
				"subject": subject,
			},
		},
		{
			name:       "MultipleConditions",
			expression: `(subject.roles.exists(r, r.name == "owner") && "admins" in subject.groups) ? duration("720h") : duration("24h")`,
			setupVars: map[string]interface{}{
				"subject": subject,
			},
		},
		{
			name:       "DurationComparison",
			expression: `defaultDuration > globalMaxDuration ? globalMaxDuration : defaultDuration`,
			setupVars: map[string]interface{}{
				"defaultDuration":   24 * time.Hour,
				"globalMaxDuration": 720 * time.Hour,
			},
		},
		{
			name:       "ComplexDurationCalculation",
			expression: `subject.roles.exists(r, r.name == "owner") ? (globalMaxDuration > duration("720h") ? duration("720h") : globalMaxDuration) : (defaultDuration < duration("1h") ? duration("1h") : defaultDuration)`,
			setupVars: map[string]interface{}{
				"subject":           subject,
				"defaultDuration":   24 * time.Hour,
				"globalMaxDuration": 720 * time.Hour,
			},
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Compile the expression once
			ast, issues := env.Compile(bm.expression)
			require.NoError(b, issues.Err())

			program, err := env.Program(ast)
			require.NoError(b, err)

			// Reset timer to exclude compilation time
			b.ResetTimer()

			// Benchmark the evaluation
			var result interface{}
			for i := 0; i < b.N; i++ {
				out, _, err := program.Eval(bm.setupVars)
				if err != nil {
					b.Fatal(err)
				}
				// Store result to prevent compiler optimization
				result = out.Value()
			}
			// Use result to ensure it's not optimized away
			if result == nil && bm.name != "Never" {
				b.Fatal("unexpected nil result")
			}
		})
	}
}

// Benchmark compilation time separately
func BenchmarkTokenLifetimeCompilation(b *testing.B) {
	env, err := cel2.NewTokenLifetimeEnvironment(cel2.EnvironmentOptions{})
	require.NoError(b, err)

	expressions := []struct {
		name string
		expr string
	}{
		{
			name: "Simple",
			expr: `subject.roles.exists(r, r.name == "owner")`,
		},
		{
			name: "Medium",
			expr: `subject.roles.exists(r, r.name == "owner") ? duration("720h") : duration("24h")`,
		},
		{
			name: "Complex",
			expr: `(subject.roles.exists(r, r.name == "owner" && r.orgID == "") || (subject.roles.exists(r, r.name == "organization-admin") && "admins" in subject.groups)) ? (globalMaxDuration > duration("720h") ? duration("720h") : globalMaxDuration) : (defaultDuration < duration("1h") ? duration("1h") : defaultDuration)`,
		},
	}

	for _, expr := range expressions {
		b.Run(expr.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ast, issues := env.Compile(expr.expr)
				if issues.Err() != nil {
					b.Fatal(issues.Err())
				}
				// Prevent compiler optimization
				_ = ast
			}
		})
	}
}

// Benchmark environment creation
func BenchmarkEnvironmentCreation(b *testing.B) {
	var env *cel.Env
	for i := 0; i < b.N; i++ {
		var err error
		env, err = cel2.NewTokenLifetimeEnvironment(cel2.EnvironmentOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
	// Use env to prevent optimization
	if env == nil {
		b.Fatal("env should not be nil")
	}
}

// BenchmarkWithResultValidation ensures results are actually computed and correct
func BenchmarkWithResultValidation(b *testing.B) {
	env, err := cel2.NewTokenLifetimeEnvironment(cel2.EnvironmentOptions{})
	require.NoError(b, err)

	subject := cel2.Subject{
		ID:     "user-123",
		Email:  "user@example.com",
		Groups: []string{"developers", "admins"},
		Roles: []cel2.Role{
			{Name: "owner", OrgID: ""},
			{Name: "member", OrgID: ""},
		},
	}

	// Compile expression
	expression := `subject.roles.exists(r, r.name == "owner") ? duration("720h") : duration("24h")`
	ast, issues := env.Compile(expression)
	require.NoError(b, issues.Err())

	program, err := env.Program(ast)
	require.NoError(b, err)

	b.ResetTimer()

	var totalNanos int64
	for i := 0; i < b.N; i++ {
		out, _, err := program.Eval(map[string]interface{}{
			"subject": subject,
		})
		if err != nil {
			b.Fatal(err)
		}

		// Extract and validate the duration result
		var dur time.Duration
		switch v := out.Value().(type) {
		case types.Duration:
			dur = v.Duration
		case time.Duration:
			dur = v
		default:
			b.Fatalf("unexpected type: %T", out.Value())
		}

		// Accumulate to prevent optimization and verify result
		totalNanos += dur.Nanoseconds()

		// Verify the result is correct (owner should get 720h)
		if dur != 720*time.Hour {
			b.Fatalf("expected 720h, got %v", dur)
		}
	}

	// Use totalNanos to ensure it's not optimized away
	if totalNanos == 0 {
		b.Fatal("total should not be zero")
	}
}

// BenchmarkDifferentInputs tests with varying inputs to prevent caching
func BenchmarkDifferentInputs(b *testing.B) {
	env, err := cel2.NewTokenLifetimeEnvironment(cel2.EnvironmentOptions{})
	require.NoError(b, err)

	expression := `subject.roles.exists(r, r.name == "owner") ? duration("720h") : duration("24h")`
	ast, issues := env.Compile(expression)
	require.NoError(b, issues.Err())

	program, err := env.Program(ast)
	require.NoError(b, err)

	// Create different subjects
	subjects := []cel2.Subject{
		{
			ID:    "user-1",
			Email: "owner@example.com",
			Roles: []cel2.Role{{Name: "owner", OrgID: ""}},
		},
		{
			ID:    "user-2",
			Email: "member@example.com",
			Roles: []cel2.Role{{Name: "member", OrgID: ""}},
		},
		{
			ID:    "user-3",
			Email: "admin@example.com",
			Roles: []cel2.Role{{Name: "admin", OrgID: ""}, {Name: "owner", OrgID: ""}},
		},
		{
			ID:    "user-4",
			Email: "guest@example.com",
			Roles: []cel2.Role{},
		},
	}

	b.ResetTimer()

	var totalNanos int64
	for i := 0; i < b.N; i++ {
		// Use different subject each iteration
		subject := subjects[i%len(subjects)]

		out, _, err := program.Eval(map[string]interface{}{
			"subject": subject,
		})
		if err != nil {
			b.Fatal(err)
		}

		// Extract duration
		var dur time.Duration
		switch v := out.Value().(type) {
		case types.Duration:
			dur = v.Duration
		case time.Duration:
			dur = v
		default:
			b.Fatalf("unexpected type: %T", out.Value())
		}

		totalNanos += dur.Nanoseconds()
	}

	// Verify we processed data
	if totalNanos == 0 {
		b.Fatal("total should not be zero")
	}
}
