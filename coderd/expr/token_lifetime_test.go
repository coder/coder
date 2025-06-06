package expr_test

import (
	"testing"
	"time"

	"github.com/expr-lang/expr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	exprtoken "github.com/coder/coder/v2/coderd/expr"
	"github.com/coder/coder/v2/coderd/rbac"
)

func TestConvertSubjectToExpr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		subject  rbac.Subject
		expected exprtoken.Subject
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
			expected: exprtoken.Subject{
				ID:     "user-123",
				Email:  "user@example.com",
				Groups: []string{"developers", "admins"},
				Roles: []exprtoken.Role{
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
				Groups: []string{"engineers"},
				Roles: rbac.RoleIdentifiers{
					rbac.RoleIdentifier{Name: "org-admin", OrganizationID: uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")},
					rbac.RoleIdentifier{Name: "member", OrganizationID: uuid.Nil},
				},
			},
			expected: exprtoken.Subject{
				ID:     "user-456",
				Email:  "user@company.com",
				Groups: []string{"engineers"},
				Roles: []exprtoken.Role{
					{Name: "org-admin", OrgID: "123e4567-e89b-12d3-a456-426614174000"},
					{Name: "member", OrgID: ""},
				},
			},
		},
		{
			name: "UserWithNoRoles",
			subject: rbac.Subject{
				ID:     "user-789",
				Email:  "guest@example.com",
				Groups: []string{},
				Roles:  rbac.RoleIdentifiers{},
			},
			expected: exprtoken.Subject{
				ID:     "user-789",
				Email:  "guest@example.com",
				Groups: []string{},
				Roles:  []exprtoken.Role{},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.subject.ExprSubject())
		})
	}
}

func TestTokenLifetimeExpressions(t *testing.T) {
	t.Parallel()

	// Create test subject
	subject := exprtoken.Subject{
		ID:     "user-123",
		Email:  "owner@company.com",
		Groups: []string{"admins"},
		Roles: []exprtoken.Role{
			{Name: "owner", OrgID: ""},
			{Name: "member", OrgID: ""},
		},
	}

	tests := []struct {
		name       string
		expression string
		vars       map[string]interface{}
		expected   time.Duration
		wantErr    bool
	}{
		{
			name:       "SimpleRoleCheck",
			expression: `any(subject.Roles, .Name == "owner") ? duration("720h") : duration("168h")`,
			vars: map[string]interface{}{
				"subject":           subject,
				"globalMaxDuration": int64(30 * 24 * time.Hour),
				"defaultDuration":   int64(24 * time.Hour),
			},
			expected: 720 * time.Hour,
		},
		{
			name:       "EmailDomainCheck",
			expression: `subject.Email endsWith "@company.com" ? duration("720h") : duration("24h")`,
			vars: map[string]interface{}{
				"subject":           subject,
				"globalMaxDuration": int64(30 * 24 * time.Hour),
				"defaultDuration":   int64(24 * time.Hour),
			},
			expected: 720 * time.Hour,
		},
		{
			name:       "GroupMembership",
			expression: `"admins" in subject.Groups ? duration("480h") : duration("24h")`,
			vars: map[string]interface{}{
				"subject":           subject,
				"globalMaxDuration": int64(30 * 24 * time.Hour),
				"defaultDuration":   int64(24 * time.Hour),
			},
			expected: 480 * time.Hour,
		},
		{
			name:       "ComplexCondition",
			expression: `(any(subject.Roles, .Name == "owner") && subject.Email endsWith "@company.com") ? duration("720h") : duration("168h")`,
			vars: map[string]interface{}{
				"subject":           subject,
				"globalMaxDuration": int64(30 * 24 * time.Hour),
				"defaultDuration":   int64(24 * time.Hour),
			},
			expected: 720 * time.Hour,
		},
		{
			name:       "UseGlobalMaxDuration",
			expression: `any(subject.Roles, .Name == "owner") ? globalMaxDuration : defaultDuration`,
			vars: map[string]interface{}{
				"subject":           subject,
				"globalMaxDuration": int64(30 * 24 * time.Hour),
				"defaultDuration":   int64(24 * time.Hour),
			},
			expected: 30 * 24 * time.Hour,
		},
		{
			name:       "DurationArithmetic",
			expression: `defaultDuration * 2`,
			vars: map[string]interface{}{
				"subject":           subject,
				"globalMaxDuration": int64(30 * 24 * time.Hour),
				"defaultDuration":   int64(24 * time.Hour),
			},
			expected: 48 * time.Hour,
		},
		{
			name:       "MinimumDuration",
			expression: `duration("1h") < duration("2h") ? duration("2h") : duration("1h")`,
			vars: map[string]interface{}{
				"subject":           subject,
				"globalMaxDuration": int64(30 * 24 * time.Hour),
				"defaultDuration":   int64(24 * time.Hour),
			},
			expected: 2 * time.Hour,
		},
		{
			name:       "AllRolesCheck",
			expression: `all(subject.Roles, .Name != "banned") ? duration("168h") : duration("1h")`,
			vars: map[string]interface{}{
				"subject":           subject,
				"globalMaxDuration": int64(30 * 24 * time.Hour),
				"defaultDuration":   int64(24 * time.Hour),
			},
			expected: 168 * time.Hour,
		},
		{
			name:       "CountRoles",
			expression: `len(subject.Roles) >= 2 ? duration("240h") : duration("24h")`,
			vars: map[string]interface{}{
				"subject":           subject,
				"globalMaxDuration": int64(30 * 24 * time.Hour),
				"defaultDuration":   int64(24 * time.Hour),
			},
			expected: 240 * time.Hour,
		},
		{
			name:       "NestedConditions",
			expression: `subject.Email endsWith "@company.com" ? (any(subject.Roles, .Name == "owner") ? duration("720h") : duration("480h")) : duration("24h")`,
			vars: map[string]interface{}{
				"subject":           subject,
				"globalMaxDuration": int64(30 * 24 * time.Hour),
				"defaultDuration":   int64(24 * time.Hour),
			},
			expected: 720 * time.Hour,
		},
		{
			name:       "InvalidDuration",
			expression: `duration("invalid")`,
			vars: map[string]interface{}{
				"subject":           subject,
				"globalMaxDuration": int64(30 * 24 * time.Hour),
				"defaultDuration":   int64(24 * time.Hour),
			},
			wantErr: true,
		},
		{
			name:       "TypeMismatch",
			expression: `"string" + 123`,
			vars: map[string]interface{}{
				"subject":           subject,
				"globalMaxDuration": int64(30 * 24 * time.Hour),
				"defaultDuration":   int64(24 * time.Hour),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			program, err := exprtoken.CompileTokenLifetimeExpression(tt.expression)
			if tt.wantErr {
				// Some errors might occur at compile time in expr
				if err != nil {
					return
				}
			} else {
				require.NoError(t, err)
			}

			result, err := expr.Run(program, tt.vars)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			// Result is int64, convert to time.Duration
			intVal, ok := result.(int64)
			require.True(t, ok, "expected int64, got %T", result)
			duration := time.Duration(intVal)
			assert.Equal(t, tt.expected, duration)
		})
	}
}

func TestExpressionCompilationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expression string
		wantErr    string
	}{
		{
			name:       "InvalidSyntax",
			expression: `subject.Roles.exists(`,
			wantErr:    "unexpected token EOF",
		},
		{
			name:       "UndefinedVariable",
			expression: `unknownVar == "test"`,
			wantErr:    "unknown name unknownVar",
		},
		{
			name:       "InvalidMethodCall",
			expression: `subject.nonexistentMethod()`,
			wantErr:    "nonexistentMethod",
		},
		// Note: expr doesn't validate function argument count at compile time
		// So we'll skip this test or handle it differently
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := exprtoken.CompileTokenLifetimeExpression(tt.expression)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func BenchmarkExprEvaluation(b *testing.B) {
	subject := exprtoken.Subject{
		ID:     "user-123",
		Email:  "owner@company.com",
		Groups: []string{"admins"},
		Roles: []exprtoken.Role{
			{Name: "owner", OrgID: ""},
			{Name: "member", OrgID: ""},
		},
	}

	expression := `any(subject.Roles, .Name == "owner") ? duration("720h") : duration("168h")`
	program, err := exprtoken.CompileTokenLifetimeExpression(expression)
	require.NoError(b, err)

	vars := map[string]interface{}{
		"subject":           subject,
		"globalMaxDuration": int64(30 * 24 * time.Hour),
		"defaultDuration":   int64(24 * time.Hour),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := expr.Run(program, vars)
		if err != nil {
			b.Fatal(err)
		}
		_ = time.Duration(result.(int64))
	}
}

func BenchmarkExprCompilation(b *testing.B) {
	expression := `any(subject.Roles, .Name == "owner") ? duration("720h") : duration("168h")`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := exprtoken.CompileTokenLifetimeExpression(expression)
		if err != nil {
			b.Fatal(err)
		}
	}
}
