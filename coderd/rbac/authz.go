package rbac

import (
	"context"
	_ "embed"

	"golang.org/x/xerrors"

	"github.com/open-policy-agent/opa/rego"
)

// RegoAuthorizer will use a prepared rego query for performing authorize()
type RegoAuthorizer struct {
	query rego.PreparedEvalQuery
}

// Load the policy from policy.rego in this directory.
//go:embed policy.rego
var policy string

func NewAuthorizer() (*RegoAuthorizer, error) {
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
	return &RegoAuthorizer{query: query}, nil
}

type authSubject struct {
	ID    string `json:"id"`
	Roles []Role `json:"roles"`
}

func (a RegoAuthorizer) Authorize(ctx context.Context, subjectID string, roles []Role, action Action, object Object) error {
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
		return ForbiddenWithInternal(xerrors.Errorf("eval rego: %w, err"), input, results)
	}

	if len(results) != 1 {
		return ForbiddenWithInternal(xerrors.Errorf("expect only 1 result, got %d", len(results)), input, results)
	}

	allowedResult, ok := (results[0].Bindings["allowed"]).(bool)
	if !ok {
		return ForbiddenWithInternal(xerrors.Errorf("expected allowed to be a bool but got %T", allowedResult), input, results)
	}

	if !allowedResult {
		return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), input, results)
	}

	return nil
}
