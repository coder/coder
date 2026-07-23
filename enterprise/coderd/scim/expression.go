package scim

import (
	"github.com/scim2/filter-parser/v2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// userQuery only supports queries of a singular attribute expression.
// Everything else is rejected. Okta just uses username.
// Eg: username eq "alice"
func userQuery(expr filter.Expression) (database.GetUsersParams, error) {
	if expr == nil {
		return database.GetUsersParams{}, nil
	}

	attrExpr, ok := expr.(*filter.AttributeExpression)
	if !ok {
		return database.GetUsersParams{}, xerrors.Errorf("expected attribute expression")
	}

	attrValue, ok := attrExpr.CompareValue.(string)
	if !ok {
		return database.GetUsersParams{}, xerrors.Errorf("expected string compare value")
	}

	var getUsers database.GetUsersParams
	switch attrExpr.AttributePath.AttributeName {
	case "userName":
		getUsers.ExactUsername = attrValue
	case "email":
		getUsers.ExactEmail = attrValue
	default:
		return database.GetUsersParams{}, xerrors.Errorf("unsupported filter attribute: %s", attrExpr.AttributePath.AttributeName)
	}

	return getUsers, nil
}
