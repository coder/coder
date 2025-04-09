package httpmw

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type chatContextKey struct{}

func ChatParam(r *http.Request) database.Chat {
	return r.Context().Value(chatContextKey{}).(database.Chat)
}

func ExtractChatParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			arg := chi.URLParam(r, "chat")
			if arg == "" {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "\"chat\" must be provided.",
				})
				return
			}
			chatID, err := uuid.Parse(arg)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Invalid chat ID.",
				})
				return
			}
			chat, err := db.GetChatByID(ctx, chatID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Failed to get chat.",
				})
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Failed to get chat.",
					Detail:  err.Error(),
				})
				return
			}
			ctx = context.WithValue(ctx, chatContextKey{}, chat)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
