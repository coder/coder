package authz

import (
	"context"
	_ "embed"

	"golang.org/x/xerrors"

	"github.com/open-policy-agent/opa/rego"
)

// regoAuthorizer will use a prepared rego query for performing authorize()
type regoAuthorizer struct {
	query rego.PreparedEvalQuery
}

// Load the policy from policy.rego in this directory.
//go:embed policy.rego
var policy string

func newAuthorizer() (*regoAuthorizer, error) {
	ctx := context.Background()
	query, err := rego.New(
		// allowed is the `allow` field from the prepared query. This is the field to check if authorization is
		// granted.
		rego.Query("allowed = data.authz.allow"),
		rego.Module("policy.rego", policy),
	).PrepareForEval(ctx)

	if err != nil {
		return nil, xerrors.Errorf("prepare query: %w", err)
	}
	return &regoAuthorizer{query: query}, nil
}

type authSubject struct {
	ID    string `json:"id"`
	Roles []Role `json:"roles"`

	SitePermissions []Permission `json:"site_permissions"`
	OrgPermissions  []Permission `json:"org_permissions"`
	UserPermissions []Permission `json:"user_permissions"`
}

func (a regoAuthorizer) Authorize(ctx context.Context, subjectID string, roles []Role, object Object, action Action) error {
	input := map[string]interface{}{
		"subject": authSubject{
			ID:    subjectID,
			Roles: roles,
		},
		"object": object,
		"action": action,
	}

	results, err := a.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return ForbiddenWithInternal(xerrors.Errorf("eval rego: %w, err"), input)
	}

	if len(results) != 1 {
		return ForbiddenWithInternal(xerrors.Errorf("expect only 1 result, got %d", len(results)), input)
	}

	if results[0].Bindings["allowed"] != true {
		return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), input)
	}
	return nil
}
