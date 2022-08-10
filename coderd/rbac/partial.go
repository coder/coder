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
			// No more work needed. An empty query is the same as
			//	'WHERE true'
			// This is likely an admin. We don't even need to use rego going
			// forward.
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

// Authorize authorizes a single object using the partially prepared queries.
func (a PartialAuthorizer) Authorize(ctx context.Context, object Object) error {
	if a.AlwaysTrue {
		return nil
	}

	// How to interpret the results of the partial queries.
	// We have a list of queries that are along the lines of:
	// 	`input.object.org_owner = ""; "me" = input.object.owner`
	//	`input.object.org_owner in {"feda2e52-8bf1-42ce-ad75-6c5595cb297a"} `
	// All these queries are joined by an 'OR'. So we need to run through each
	// query, and evaluate it.
	//
	// In each query, we have a list of the evaluation results, which should be
	// all boolean expressions. In the above 1st example, there are 2 boolean
	// expressions. These expressions within a single query are `AND` together by rego.
EachQueryLoop:
	for _, q := range a.PreparedQueries {
		// We need to eval each query with the newly known fields.
		results, err := q.Eval(ctx, rego.EvalInput(map[string]interface{}{
			"object": object,
		}))
		if err != nil {
			continue EachQueryLoop
		}

		// If there are no results, then the query is false. This is because rego
		// treats false queries as "undefined". So if any expression is false, the
		// result is an empty list.
		if len(results) == 0 {
			continue EachQueryLoop
		}

		// If there is more than 1 result, that means there is more than 1 rule.
		// This should not happen, because our query should always be an expression.
		// If this every occurs, it is likely the original query was not an expression.
		if len(results) > 1 {
			continue EachQueryLoop
		}

		// Our queries should be simple, and should not yield any bindings.
		// A binding is something like 'x := 1'. This binding as an expression is
		// 'true', but in our case is unhelpful. We are not analyzing this ast to
		// map bindings. So just error out. Similar to above, our queries should
		// always be boolean expressions.
		if len(results[0].Bindings) > 0 {
			continue EachQueryLoop
		}

		// We have a valid set of boolean expressions! All expressions are 'AND'd
		// together. This is automatic by rego, so we should not actually need to
		// inspect this any further. But just in case, we will verify each expression
		// did resolve to 'true'. This is purely defensive programming.
		for _, exp := range results[0].Expressions {
			if exp.String() != "true" {
				continue EachQueryLoop
			}
		}

		return nil
	}

	return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), a.Input, nil)
}
