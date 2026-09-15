package httpmw

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type chatProjectParamContextKey struct{}

// ChatProjectParam returns the chat project from the ExtractChatProjectParam handler.
func ChatProjectParam(r *http.Request) database.ChatProject {
	project, ok := r.Context().Value(chatProjectParamContextKey{}).(database.ChatProject)
	if !ok {
		panic("developer error: chat project param middleware not provided")
	}
	return project
}

// ExtractChatProjectParam grabs a chat project from the "project" URL parameter.
func ExtractChatProjectParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			projectID, parsed := ParseUUIDParam(rw, r, "project")
			if !parsed {
				return
			}

			// Route extraction resolves identity before the handler authorizes its action.
			//nolint:gocritic // Restrict system access to the identity lookup.
			project, err := db.GetChatProjectByID(dbauthz.AsSystemRestricted(ctx), projectID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching chat project.",
					Detail:  err.Error(),
				})
				return
			}

			ctx = context.WithValue(ctx, chatProjectParamContextKey{}, project)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
