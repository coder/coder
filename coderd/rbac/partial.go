package rbac

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/open-policy-agent/opa/rego"
)

type PartialAuthorizer struct {
	// PartialQueries is mainly used for unit testing.
	PartialQueries  *rego.PartialQueries
	PreparedQueries []rego.PreparedEvalQuery
	AlwaysTrue      bool
	Input           map[string]interface{}
}

func newPartialAuthorizer(ctx context.Context, subjectID string, roles []Role, action Action, objectType string) (*PartialAuthorizer, error) {
	input := map[string]interface{}{
		"subject": authSubject{
			ID:    subjectID,
			Roles: roles,
		},
		"object": map[string]string{
			"type": objectType,
		},
		"action": action,
	}

	partialQueries, err := rego.New(
		rego.Query("true = data.authz.allow"),
		rego.Module("policy.rego", policy),
		rego.Unknowns([]string{
			"input.object.owner",
			"input.object.org_owner",
		}),
		rego.Input(input),
	).Partial(ctx)
	if err != nil {
		return nil, xerrors.Errorf("prepare: %w", err)
	}

	pAuth := &PartialAuthorizer{
		PartialQueries:  partialQueries,
		PreparedQueries: []rego.PreparedEvalQuery{},
		Input:           input,
	}

	preparedQueries := make([]rego.PreparedEvalQuery, 0, len(partialQueries.Queries))
	for _, q := range partialQueries.Queries {
		if q.String() == "" {
			// No more work needed, this will always be true,
			pAuth.AlwaysTrue = true
			preparedQueries = []rego.PreparedEvalQuery{}
			break
		}
		results, err := rego.New(
			rego.ParsedQuery(q),
		).PrepareForEval(ctx)
		if err != nil {
			return nil, xerrors.Errorf("prepare query %s: %w", q.String(), err)
		}
		preparedQueries = append(preparedQueries, results)
	}
	pAuth.PreparedQueries = preparedQueries

	return pAuth, nil
}

// Authorize authorizes a single object using teh partially prepared queries.
func (a PartialAuthorizer) Authorize(ctx context.Context, object Object) error {
	if a.AlwaysTrue {
		return nil
	}

EachQueryLoop:
	for _, q := range a.PreparedQueries {
		// We need to eval each query with the newly known fields.
		results, err := q.Eval(ctx, rego.EvalInput(map[string]interface{}{
			"object": object,
		}))
		if err != nil {
			continue EachQueryLoop
		}

		// The below code is intended to fail safe. We only support queries that
		// return simple results.

		// 0 results means the query is false.
		if len(results) == 0 {
			continue EachQueryLoop
		}

		// We should never get more than 1 result
		if len(results) > 1 {
			continue EachQueryLoop
		}

		// All queries should resolve, we should not have bindings
		if len(results[0].Bindings) > 0 {
			continue EachQueryLoop
		}

		for _, exp := range results[0].Expressions {
			// Any other "true" expressions that are not "true" are not expected.
			if exp.String() != "true" {
				continue EachQueryLoop
			}
		}

		return nil
	}

	return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), a.Input, nil)
}
