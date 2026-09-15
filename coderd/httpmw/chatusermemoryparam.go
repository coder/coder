package httpmw

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type chatUserMemoryParamContextKey struct{}

// ChatUserMemoryParam returns the chat user memory from the extractor.
func ChatUserMemoryParam(r *http.Request) database.GetChatUserMemoryByIDRow {
	memory, ok := r.Context().Value(chatUserMemoryParamContextKey{}).(database.GetChatUserMemoryByIDRow)
	if !ok {
		panic("developer error: chat user memory param middleware not provided")
	}
	return memory
}

// ExtractChatUserMemoryParam resolves a chat user memory from "memory".
func ExtractChatUserMemoryParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			memoryID, parsed := ParseUUIDParam(rw, r, "memory")
			if !parsed {
				return
			}

			// Route extraction resolves identity before the handler authorizes its action.
			//nolint:gocritic // Restrict system access to the identity lookup.
			memory, err := db.GetChatUserMemoryByID(dbauthz.AsSystemRestricted(ctx), memoryID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching chat user memory.",
					Detail:  err.Error(),
				})
				return
			}
			ctx = context.WithValue(ctx, chatUserMemoryParamContextKey{}, memory)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
