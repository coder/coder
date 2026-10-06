package httpmw

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type chatProjectMemoryParamContextKey struct{}

// ChatProjectMemoryParam returns the chat project memory from the extractor.
func ChatProjectMemoryParam(r *http.Request) database.GetChatProjectMemoryByIDRow {
	memory, ok := r.Context().Value(chatProjectMemoryParamContextKey{}).(database.GetChatProjectMemoryByIDRow)
	if !ok {
		panic("developer error: chat project memory param middleware not provided")
	}
	return memory
}

// ExtractChatProjectMemoryParam resolves a chat project memory from "memory".
func ExtractChatProjectMemoryParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			memoryID, parsed := ParseUUIDParam(rw, r, "memory")
			if !parsed {
				return
			}

			// Route extraction resolves identity before the handler authorizes its action.
			//nolint:gocritic // Restrict system access to the identity lookup.
			memory, err := db.GetChatProjectMemoryByID(dbauthz.AsSystemRestricted(ctx), memoryID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching chat project memory.",
					Detail:  err.Error(),
				})
				return
			}
			ctx = context.WithValue(ctx, chatProjectMemoryParamContextKey{}, memory)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
