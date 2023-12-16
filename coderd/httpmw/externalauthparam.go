package httpmw

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
)

type externalAuthParamContextKey struct{}

func ExternalAuthParam(r *http.Request) *externalauth.Config {
	config, ok := r.Context().Value(externalAuthParamContextKey{}).(*externalauth.Config)
	if !ok {
		panic("developer error: external auth param middleware not provided")
	}
	return config
}

func ExtractExternalAuthParam(configs []*externalauth.Config) func(next http.Handler) http.Handler {
	configByID := make(map[string]*externalauth.Config)
	for _, c := range configs {
		configByID[c.ID] = c
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			config, ok := configByID[chi.URLParam(r, "externalauth")]
			if !ok {
				httpapi.ResourceNotFound(w)
				return
			}

			r = r.WithContext(context.WithValue(r.Context(), externalAuthParamContextKey{}, config))
			next.ServeHTTP(w, r)
		})
	}
}
