package httpmw

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

func RequireConnectionKey(db database.Store) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v := r.Header.Get(codersdk.SessionTokenHeader)
			if v == "" {
				httpapi.Write(r.Context(), w, http.StatusUnauthorized, codersdk.Response{
					Message: "missing connection key",
				})
				return
			}

			key, err := db.GetDebugHealthConnectionKey(r.Context())
			if err != nil {
				httpapi.Write(r.Context(), w, http.StatusInternalServerError, codersdk.Response{
					Message: "failed to get connection key",
					Detail:  err.Error(),
				})
				return
			}

			if v != key {
				httpapi.Write(r.Context(), w, http.StatusUnauthorized, codersdk.Response{
					Message: "invalid connection key",
				})
				return
			}

			h.ServeHTTP(w, r)
		})
	}
}
