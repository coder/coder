package rbac

import (
	"context"
	"strings"

	"github.com/coder/coder/coderd/rbac/regosql"

	"github.com/coder/coder/coderd/rbac/regosql/sqltypes"

	"golang.org/x/xerrors"
)

type AuthorizeFilter interface {
	SQLString() string
}

type authorizedSQLFilter struct {
	sqlString string
	auth      *PartialAuthorizer
}

func ConfigWithACL() regosql.ConvertConfig {
	return regosql.ConvertConfig{
		VariableConverter: regosql.DefaultVariableConverter(),
	}
}

func ConfigWithoutACL() regosql.ConvertConfig {
	return regosql.ConvertConfig{
		VariableConverter: regosql.NoACLConverter(),
	}
}

func Compile(cfg regosql.ConvertConfig, pa *PartialAuthorizer) (AuthorizeFilter, error) {
	root, err := regosql.ConvertRegoAst(cfg, pa.partialQueries)
	if err != nil {
		return nil, xerrors.Errorf("convert rego ast: %w", err)
	}

	// Generate the SQL
	gen := sqltypes.NewSQLGenerator()
	sqlString := root.SQLString(gen)
	if len(gen.Errors()) > 0 {
		var errStrings []string
		for _, err := range gen.Errors() {
			errStrings = append(errStrings, err.Error())
		}
		return nil, xerrors.Errorf("sql generation errors: %v", strings.Join(errStrings, ", "))
	}

	return &authorizedSQLFilter{
		sqlString: sqlString,
		auth:      pa,
	}, nil
}

func (a *authorizedSQLFilter) Eval(object Object) bool {
	return a.auth.Authorize(context.Background(), object) == nil
}

func (a *authorizedSQLFilter) SQLString() string {
	return a.sqlString
}
