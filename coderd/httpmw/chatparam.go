package httpmw

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/codersdk"
)

type chatParamContextKey struct{}

// ChatParam returns the chat from the ExtractChatParam handler.
func ChatParam(r *http.Request) database.Chat {
	chat, ok := r.Context().Value(chatParamContextKey{}).(database.Chat)
	if !ok {
		panic("developer error: chat param middleware not provided")
	}
	return chat
}

// FetchChat returns the chat as the actor in ctx sees it. A missing chat, or
// a chat the actor may not read, gives a 404. Every error implements
// httperror.Responder.
func FetchChat(ctx context.Context, db database.Store, chatID uuid.UUID) (database.Chat, error) {
	chat, err := db.GetChatByID(ctx, chatID)
	if httpapi.Is404Error(err) {
		return database.Chat{}, httperror.WrapResponseError(http.StatusNotFound, httpapi.ResourceNotFoundResponse, err)
	}
	if err != nil {
		return database.Chat{}, httperror.WrapResponseError(http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching chat.",
			Detail:  err.Error(),
		}, err)
	}
	return chat, nil
}

// ExtractChatParam grabs a chat from the "chat" URL parameter.
func ExtractChatParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			chatID, parsed := ParseUUIDParam(rw, r, "chat")
			if !parsed {
				return
			}

			chat, err := FetchChat(ctx, db, chatID)
			if err != nil {
				httperror.WriteResponseError(ctx, rw, err)
				return
			}

			ctx = context.WithValue(ctx, chatParamContextKey{}, chat)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
