package httpmw

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/httpapi"
)

type gitAuthParamContextKey struct{}

func GitAuthParam(r *http.Request) *gitauth.Config {
	config, ok := r.Context().Value(gitAuthParamContextKey{}).(*gitauth.Config)
	if !ok {
		panic("developer error: gitauth param middleware not provided")
	}
	return config
}

func ExtractGitAuthParam(configs []*gitauth.Config) func(next http.Handler) http.Handler {
	configByID := make(map[string]*gitauth.Config)
	for _, c := range configs {
		configByID[c.ID] = c
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			config, ok := configByID[chi.URLParam(r, "gitauth")]
			if !ok {
				httpapi.ResourceNotFound(w)
				return
			}

			r = r.WithContext(context.WithValue(r.Context(), gitAuthParamContextKey{}, config))
			next.ServeHTTP(w, r)
		})
	}
}
