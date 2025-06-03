package cel

import (
	"reflect"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/ext"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/rbac"
)

// Subject is a simplified version of rbac.Subject for CEL evaluation
type Subject struct {
	ID     string   `cel:"id"`
	Email  string   `cel:"email"`
	Groups []string `cel:"groups"`
	Roles  []Role   `cel:"roles"`
}

// Role represents a role with optional organization scope
type Role struct {
	Name  string `cel:"name"`
	OrgID string `cel:"orgID"` // Empty string for site-wide roles
}

// ConvertSubjectToCEL converts an rbac.Subject to a CEL-friendly format
func ConvertSubjectToCEL(subject rbac.Subject) Subject {
	celRoles := make([]Role, 0, len(subject.SafeRoleNames()))
	for _, role := range subject.SafeRoleNames() {
		// Convert zero UUID (site-wide roles) to empty string for easier CEL expression writing
		orgID := role.OrganizationID.String()
		if orgID == uuid.Nil.String() {
			orgID = ""
		}

		celRoles = append(celRoles, Role{
			Name:  role.Name,
			OrgID: orgID,
		})
	}

	return Subject{
		ID:     subject.ID,
		Email:  subject.Email,
		Groups: subject.Groups,
		Roles:  celRoles,
	}
}

// EnvironmentOptions allows customization of the CEL environment
type EnvironmentOptions struct {
	// ExtraOptions additional CEL environment options
	ExtraOptions []cel.EnvOption
}

// NewTokenLifetimeEnvironment creates a CEL environment for token lifetime expressions
// with common variables and functions. Can be customized with EnvironmentOptions.
func NewTokenLifetimeEnvironment(opts EnvironmentOptions) (*cel.Env, error) {
	envOptions := []cel.EnvOption{
		// Register native types - enable parsing of struct tags
		ext.NativeTypes(
			ext.ParseStructTags(true),
			reflect.TypeFor[Subject](),
			reflect.TypeFor[Role](),
		),
		cel.Variable("subject", cel.ObjectType("cel.Subject")),
		cel.Variable("globalMaxDuration", cel.DurationType),
		cel.Variable("defaultDuration", cel.DurationType),
		DurationFunction(),
	}

	// Add any extra options
	envOptions = append(envOptions, opts.ExtraOptions...)

	return cel.NewEnv(envOptions...)
}

// DurationFunction creates a CEL function for parsing duration strings
func DurationFunction() cel.EnvOption {
	return cel.Function("duration",
		cel.Overload("string_to_duration", []*cel.Type{cel.StringType}, cel.DurationType,
			cel.UnaryBinding(func(value ref.Val) ref.Val {
				str, ok := value.(types.String)
				if !ok {
					return types.NewErr("invalid string value")
				}
				if d, err := time.ParseDuration(string(str)); err == nil {
					return types.Duration{Duration: d}
				}
				return types.NewErr("invalid duration format: %s", string(str))
			})))
}

// TODO: Add timeout protection for CEL evaluation in future iterations
// Example: EvaluateWithTimeout(ctx, program, vars, 100*time.Millisecond)

// TODO: Add expression complexity limits in future iterations
// Example: Check AST node count or expression depth
