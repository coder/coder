package sqlxqueries

import (
	"context"

	"github.com/jmoiron/sqlx"

	"golang.org/x/xerrors"
)

// constructQuery will return a SQL query by the given template name.
// It will also return the arguments in order for the query based on the input
// argument.
func constructQuery(queryName string, argument any) (string, []any, error) {
	// No argument was given, use an empty struct.
	if argument == nil {
		argument = struct{}{}
	}

	query, err := query(queryName, argument)
	if err != nil {
		return "", nil, xerrors.Errorf("get query: %w", err)
	}

	query, args, err := bindNamed(query, argument)
	if err != nil {
		return "", nil, xerrors.Errorf("bind named: %w", err)
	}
	return query, args, nil
}

// SelectContext runs the named query on the given database.
// If the query returns no rows, an empty slice is returned.
func SelectContext[RT any](ctx context.Context, q sqlx.QueryerContext, queryName string, argument any) ([]RT, error) {
	var empty []RT

	query, args, err := constructQuery(queryName, argument)
	if err != nil {
		return empty, xerrors.Errorf("get query: %w", err)
	}

	err = sqlx.SelectContext(ctx, q, &empty, query, args...)
	if err != nil {
		return empty, xerrors.Errorf("%s: %w", queryName, err)
	}

	return empty, nil
}

// GetContext runs the named query on the given database.
// If the query returns no rows, sql.ErrNoRows is returned.
func GetContext[RT any](ctx context.Context, q sqlx.QueryerContext, queryName string, argument interface{}) (RT, error) {
	var empty RT

	query, args, err := constructQuery(queryName, argument)
	if err != nil {
		return empty, xerrors.Errorf("get query: %w", err)
	}

	// GetContext maps the results of the query to the items slice by struct
	// db tags.
	err = sqlx.GetContext(ctx, q, &empty, query, args...)
	if err != nil {
		return empty, xerrors.Errorf("%s: %w", queryName, err)
	}

	return empty, nil
}
