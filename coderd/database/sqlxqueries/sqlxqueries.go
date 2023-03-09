package sqlxqueries

import (
	"bytes"
	"embed"
	"sync"
	"text/template"

	"golang.org/x/xerrors"
)

//go:embed *.gosql
var sqlxQueries embed.FS

var (
	// Only parse the queries once.
	once   sync.Once
	cached *template.Template
)

// loadQueries parses the embedded queries and returns the template.
// Results are cached.
func loadQueries() *template.Template {
	once.Do(func() {
		tpls, err := template.New("").
			Funcs(template.FuncMap{
				"int32": func(i int) int32 { return int32(i) },
			}).ParseFS(sqlxQueries, "*.gosql")
		if err != nil {
			panic(xerrors.Errorf("developer error parse sqlx queries: %w", err))
			return
		}
		cached = tpls
	})

	return cached
}

// query executes the named template with the given data and returns the result.
// The returned query string is SQL.
func query(name string, data interface{}) (string, error) {
	tpls := loadQueries()

	var out bytes.Buffer
	err := tpls.ExecuteTemplate(&out, name, data)
	if err != nil {
		return "", xerrors.Errorf("execute template %s: %w", name, err)
	}
	return out.String(), nil
}
