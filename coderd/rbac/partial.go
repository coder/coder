package rbac

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"

	"github.com/coder/coder/coderd/tracing"
)

type PartialAuthorizer struct {
	// partialQueries is mainly used for unit testing to assert our rego policy
	// can always be compressed into a set of queries.
	partialQueries *rego.PartialQueries
	// input is used purely for debugging and logging.
	input map[string]interface{}
	// preparedQueries are the compiled set of queries after partial evaluation.
	// Cache these prepared queries to avoid re-compiling the queries.
	// If alwaysTrue is true, then ignore these.
	preparedQueries []rego.PreparedEvalQuery
	// alwaysTrue is if the subject can always perform the action on the
	// resource type, regardless of the unknown fields.
	alwaysTrue bool
}

var _ PreparedAuthorized = (*PartialAuthorizer)(nil)

func (pa *PartialAuthorizer) Compile() (AuthorizeFilter, error) {
	filter, err := Compile(pa)
	if err != nil {
		return nil, xerrors.Errorf("compile: %w", err)
	}
	return filter, nil
}

func (pa *PartialAuthorizer) Authorize(ctx context.Context, object Object) error {
	if pa.alwaysTrue {
		return nil
	}

	// No queries means always false
	if len(pa.preparedQueries) == 0 {
		return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), pa.input, nil)
	}

	parsed, err := ast.InterfaceToValue(map[string]interface{}{
		"object": object,
	})
	if err != nil {
		return xerrors.Errorf("parse object: %w", err)
	}

	// How to interpret the results of the partial queries.
	// We have a list of queries that are along the lines of:
	// 	`input.object.org_owner = ""; "me" = input.object.owner`
	//	`input.object.org_owner in {"feda2e52-8bf1-42ce-ad75-6c5595cb297a"} `
	// All these queries are joined by an 'OR'. So we need to run through each
	// query, and evaluate it.
	//
	// In each query, we have a list of the expressions, which should be
	// all boolean expressions. In the above 1st example, there are 2.
	// These expressions within a single query are `AND` together by rego.
EachQueryLoop:
	for _, q := range pa.preparedQueries {
		// We need to eval each query with the newly known fields.
		results, err := q.Eval(ctx, rego.EvalParsedInput(parsed))
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
			if v, ok := exp.Value.(bool); !ok || !v {
				continue EachQueryLoop
			}
		}

		return nil
	}

	return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"), pa.input, nil)
}

func newPartialAuthorizer(ctx context.Context, subjectID string, roles []Role, scope Role, groups []string, action Action, objectType string) (*PartialAuthorizer, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	input := map[string]interface{}{
		"subject": authSubject{
			ID:     subjectID,
			Roles:  roles,
			Scope:  scope,
			Groups: groups,
		},
		"object": map[string]string{
			"type": objectType,
		},
		"action": action,
	}

	// Run the rego policy with a few unknown fields. This should simplify our
	// policy to a set of queries.
	partialQueries, err := rego.New(
		rego.Query("data.authz.allow = true"),
		rego.Module("policy.rego", policy),
		rego.Unknowns([]string{
			"input.object.owner",
			"input.object.org_owner",
			"input.object.acl_user_list",
			"input.object.acl_group_list",
		}),
		rego.Input(input),
	).Partial(ctx)
	if err != nil {
		return nil, xerrors.Errorf("prepare: %w", err)
	}

	pAuth := &PartialAuthorizer{
		partialQueries:  partialQueries,
		preparedQueries: []rego.PreparedEvalQuery{},
		input:           input,
	}

	// Prepare each query to optimize the runtime when we iterate over the objects.
	preparedQueries := make([]rego.PreparedEvalQuery, 0, len(partialQueries.Queries))
	for _, q := range partialQueries.Queries {
		if q.String() == "" {
			// No more work needed. An empty query is the same as
			//	'WHERE true'
			// This is likely an admin. We don't even need to use rego going
			// forward.
			pAuth.alwaysTrue = true
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
	pAuth.preparedQueries = preparedQueries

	return pAuth, nil
}
