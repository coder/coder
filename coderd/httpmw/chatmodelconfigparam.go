package httpmw

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type chatModelConfigParamContextKey struct{}

// ChatModelConfigParam returns the chat model config from the
// ExtractChatModelConfigParam middleware.
func ChatModelConfigParam(r *http.Request) database.ChatModelConfig {
	config, ok := r.Context().Value(chatModelConfigParamContextKey{}).(database.ChatModelConfig)
	if !ok {
		panic("developer error: chat model config param middleware not provided")
	}
	return config
}

// ExtractChatModelConfigParam grabs a chat model config from the "model" URL
// parameter, authorizes reading it, and re-injects both the config and its
// organization into the route context. It authorizes READ ONLY: write
// authorization happens in dbauthz inside the handler's write transaction, so
// a caller who may read but not write still reaches the handler (audit
// initialization records the denied write there).
func ExtractChatModelConfigParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			modelID, parsed := ParseUUIDParam(rw, r, "model")
			if !parsed {
				return
			}

			config, err := db.GetChatModelConfigByID(r.Context(), modelID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching chat model config.",
					Detail:  err.Error(),
				})
				return
			}

			ctx = context.WithValue(ctx, chatModelConfigParamContextKey{}, config)
			chi.RouteContext(ctx).URLParams.Add("organization", config.OrganizationID.String())
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
