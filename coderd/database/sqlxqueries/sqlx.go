package sqlxqueries

import (
	"context"

	"github.com/jmoiron/sqlx"

	"golang.org/x/xerrors"
)

// SelectContext runs the named query on the given database.
// If the query returns no rows, an empty slice is returned.
func SelectContext[RT any](ctx context.Context, q sqlx.QueryerContext, queryName string, argument interface{}) ([]RT, error) {
	var empty []RT

	query, err := query(queryName, nil)
	if err != nil {
		return empty, xerrors.Errorf("get query: %w", err)
	}

	query, args, err := bindNamed(query, argument)
	if err != nil {
		return empty, xerrors.Errorf("bind named: %w", err)
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

	query, err := query(queryName, argument)
	if err != nil {
		return empty, xerrors.Errorf("get query: %w", err)
	}

	query, args, err := bindNamed(query, argument)
	if err != nil {
		return empty, xerrors.Errorf("bind named: %w", err)
	}

	// GetContext maps the results of the query to the items slice by struct
	// db tags.
	err = sqlx.GetContext(ctx, q, &empty, query, args...)
	if err != nil {
		return empty, xerrors.Errorf("%s: %w", queryName, err)
	}

	return empty, nil
}
