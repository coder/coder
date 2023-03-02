package database

import (
	"context"

	"github.com/coder/coder/coderd/database/sqlxqueries"
	"golang.org/x/xerrors"
)

func sqlxSelect[RT any](ctx context.Context, q *sqlQuerier, queryName string, argument interface{}) ([]RT, error) {
	var empty []RT

	query, err := sqlxqueries.Query(queryName, nil)
	if err != nil {
		return empty, xerrors.Errorf("get query: %w", err)
	}

	query, args, err := bindNamed(query, argument)
	if err != nil {
		return empty, xerrors.Errorf("bind named: %w", err)
	}

	// GetContext maps the results of the query to the items slice by struct
	// db tags.
	err = q.sdb.SelectContext(ctx, &empty, query, args...)
	if err != nil {
		return empty, xerrors.Errorf("%s: %w", queryName, err)
	}

	return empty, nil
}

func sqlxGet[RT any](ctx context.Context, q *sqlQuerier, queryName string, argument interface{}) (RT, error) {
	var empty RT

	query, err := sqlxqueries.Query(queryName, nil)
	if err != nil {
		return empty, xerrors.Errorf("get query: %w", err)
	}

	query, args, err := bindNamed(query, argument)
	if err != nil {
		return empty, xerrors.Errorf("bind named: %w", err)
	}

	// GetContext maps the results of the query to the items slice by struct
	// db tags.
	err = q.sdb.GetContext(ctx, &empty, query, args...)
	if err != nil {
		return empty, xerrors.Errorf("%s: %w", queryName, err)
	}

	return empty, nil
}
