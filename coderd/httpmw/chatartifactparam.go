package httpmw

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type chatArtifactParamContextKey struct{}

// ChatArtifactParam returns the chat artifact from the extractor.
func ChatArtifactParam(r *http.Request) database.GetChatArtifactByIDRow {
	artifact, ok := r.Context().Value(chatArtifactParamContextKey{}).(database.GetChatArtifactByIDRow)
	if !ok {
		panic("developer error: chat artifact param middleware not provided")
	}
	return artifact
}

// ExtractChatArtifactParam resolves a chat artifact from "artifact" as the
// caller. A caller who cannot read the artifact gets 404.
func ExtractChatArtifactParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			artifactID, parsed := ParseUUIDParam(rw, r, "artifact")
			if !parsed {
				return
			}
			artifact, err := db.GetChatArtifactByID(ctx, artifactID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching chat artifact.",
					Detail:  err.Error(),
				})
				return
			}
			ctx = context.WithValue(ctx, chatArtifactParamContextKey{}, artifact)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
