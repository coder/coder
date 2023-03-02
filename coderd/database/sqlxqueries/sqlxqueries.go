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

func Query(name string, data interface{}) (string, error) {
	tpls, err := queries()
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	// TODO: Should we cache these?
	err = tpls.ExecuteTemplate(&out, name, data)
	if err != nil {
		return "", xerrors.Errorf("execute template %s: %w", name, err)
	}
	return out.String(), nil
}

func GetWorkspaceBuildByID() (string, error) {
	return Query("GetWorkspaceBuildByID", nil)
}
