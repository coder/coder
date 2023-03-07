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
	once        sync.Once
	cached      *template.Template
	cachedError error
)

func queries() (*template.Template, error) {
	once.Do(func() {
		tpls, err := template.ParseFS(sqlxQueries, "*.gosql")
		if err != nil {
			cachedError = xerrors.Errorf("developer error parse sqlx queries: %w", err)
		}
		cached = tpls
	})

	return cached, cachedError
}

// query executes the named template with the given data and returns the result.
// The returned query string is SQL.
func query(name string, data interface{}) (string, error) {
	tpls, err := queries()
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	err = tpls.ExecuteTemplate(&out, name, data)
	if err != nil {
		return "", xerrors.Errorf("execute template %s: %w", name, err)
	}
	return out.String(), nil
}
