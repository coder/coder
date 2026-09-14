package sqltypes

type SQLGenerator struct {
	errors []error
}

func NewSQLGenerator() *SQLGenerator {
	return &SQLGenerator{}
}

func (g *SQLGenerator) AddError(err error) {
	if err != nil {
		g.errors = append(g.errors, err)
	}
}

func (g *SQLGenerator) Errors() []error {
	return g.errors
}
