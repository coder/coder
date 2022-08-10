package rbac

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/open-policy-agent/opa/rego"
)

type PartialAuthorizer struct {
	// PartialRego is mainly used for unit testing. It is the rego source policy.
	PartialRego     *rego.Rego
	PartialQueries  *rego.PartialQueries
	PreparedQueries []rego.PreparedEvalQuery
	AlwaysTrue      bool
	Input           map[string]interface{}
}

func newPartialAuthorizer(ctx context.Context, _ *rego.Rego, input map[string]interface{}) (*PartialAuthorizer, error) {
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

	alwaysTrue := false
	preparedQueries := make([]rego.PreparedEvalQuery, 0, len(partialQueries.Queries))
	for _, q := range partialQueries.Queries {
		if q.String() == "" {
			alwaysTrue = true
			continue
		}
		results, err := rego.New(
			rego.ParsedQuery(q),
		).PrepareForEval(ctx)
		if err != nil {
			return nil, xerrors.Errorf("prepare query %s: %w", q.String(), err)
		}
		preparedQueries = append(preparedQueries, results)
	}

	return &PartialAuthorizer{
		PartialQueries:  partialQueries,
		PreparedQueries: preparedQueries,
		Input:           input,
		AlwaysTrue:      alwaysTrue,
	}, nil
}

// Authorize authorizes a single object using teh partially prepared queries.
func (a PartialAuthorizer) Authorize(ctx context.Context, object Object) error {
	if a.AlwaysTrue {
		return nil
	}

EachQueryLoop:
	for _, q := range a.PreparedQueries {
		results, err := q.Eval(ctx, rego.EvalInput(map[string]interface{}{
			"object": object,
		}))
		if err != nil {
			continue EachQueryLoop
		}

		if len(results) == 0 {
			continue EachQueryLoop
		}

		if len(results) > 1 {
			continue EachQueryLoop
		}

		if len(results[0].Bindings) > 0 {
			continue EachQueryLoop
		}

		for _, exp := range results[0].Expressions {
			if exp.String() != "true" {
				continue EachQueryLoop
			}
		}

		return nil
	}

	return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), a.Input, nil)
}
