package httpmw

import (
	"github.com/go-chi/cors"
	"net/http"
)

func CorsMW(allowAll bool, origins ...string) func(next http.Handler) http.Handler {
	if allowAll {
		origins = []string{"*"}
	}
	return cors.Handler(cors.Options{
		AllowedOrigins: origins,
		// We only need GET for latency requests
		AllowedMethods: []string{http.MethodOptions, http.MethodGet},
		AllowedHeaders: []string{"Accept", "Content-Type", "X-LATENCY-CHECK", "X-CSRF-TOKEN"},
		// Do not send any cookies
		AllowCredentials: false,
	})
}
