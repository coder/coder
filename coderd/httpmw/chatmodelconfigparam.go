package httpmw

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
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
// parameter after the organization route parameter has been resolved. It only
// resolves route identity; each handler authorizes its own action.
func ExtractChatModelConfigParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			modelID, parsed := ParseUUIDParam(rw, r, "model")
			if !parsed {
				return
			}

			organization := OrganizationParam(r)
			//nolint:gocritic // Each model handler authorizes its concrete action.
			config, err := db.GetChatModelConfigByID(dbauthz.AsSystemRestricted(ctx), modelID)
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
			if config.OrganizationID != organization.ID {
				httpapi.ResourceNotFound(rw)
				return
			}

			ctx = context.WithValue(ctx, chatModelConfigParamContextKey{}, config)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
