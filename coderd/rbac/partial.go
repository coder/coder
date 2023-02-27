package rbac

import (
	"context"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac/regosql"
	"github.com/coder/coder/coderd/tracing"
)

type PartialAuthorizer struct {
	// partialQueries is mainly used for unit testing to assert our rego policy
	// can always be compressed into a set of queries.
	partialQueries *rego.PartialQueries

	// input is used purely for debugging and logging.
	subjectInput        Subject
	subjectAction       Action
	subjectResourceType Object

	// preparedQueries are the compiled set of queries after partial evaluation.
	// Cache these prepared queries to avoid re-compiling the queries.
	// If alwaysTrue is true, then ignore these.
	preparedQueries []rego.PreparedEvalQuery
	// alwaysTrue is if the subject can always perform the action on the
	// resource type, regardless of the unknown fields.
	alwaysTrue bool
}

var _ PreparedAuthorized = (*PartialAuthorizer)(nil)

func (pa *PartialAuthorizer) CompileToSQL(ctx context.Context, cfg regosql.ConvertConfig) (string, error) {
	_, span := tracing.StartSpan(ctx, trace.WithAttributes(
		// Query count is a rough indicator of the complexity of the query
		// that needs to be converted into SQL.
		attribute.Int("query_count", len(pa.preparedQueries)),
		attribute.Bool("always_true", pa.alwaysTrue),
	))
	defer span.End()

	filter, err := Compile(cfg, pa)
	if err != nil {
		return "", xerrors.Errorf("compile: %w", err)
	}
	return filter.SQLString(), nil
}

func (pa *PartialAuthorizer) Authorize(ctx context.Context, object Object) error {
	if pa.alwaysTrue {
		return nil
	}

	// If we have no queries, then no queries can return 'true'.
	// So the result is always 'false'.
	if len(pa.preparedQueries) == 0 {
		return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"),
			pa.subjectInput, pa.subjectAction, pa.subjectResourceType, nil)
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

	return ForbiddenWithInternal(xerrors.Errorf("policy disallows request"),
		pa.subjectInput, pa.subjectAction, pa.subjectResourceType, nil)
}

func (a RegoAuthorizer) newPartialAuthorizer(ctx context.Context, subject Subject, action Action, objectType string) (*PartialAuthorizer, error) {
	if subject.Roles == nil {
		return nil, xerrors.Errorf("subject must have roles")
	}
	if subject.Scope == nil {
		return nil, xerrors.Errorf("subject must have a scope")
	}

	input, err := regoPartialInputValue(subject, action, objectType)
	if err != nil {
		return nil, xerrors.Errorf("prepare input: %w", err)
	}

	partialQueries, err := a.partialQuery.Partial(ctx, rego.EvalParsedInput(input))
	if err != nil {
		return nil, xerrors.Errorf("prepare: %w", err)
	}

	pAuth := &PartialAuthorizer{
		partialQueries:  partialQueries,
		preparedQueries: []rego.PreparedEvalQuery{},
		subjectInput:    subject,
		subjectResourceType: Object{
			Type: objectType,
			ID:   "prepared-object",
		},
		subjectAction: action,
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
