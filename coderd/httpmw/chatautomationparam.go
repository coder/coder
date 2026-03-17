package httpmw

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type chatAutomationParamContextKey struct{}

// ChatAutomationParam returns the chat automation from the
// ExtractChatAutomationParam handler.
func ChatAutomationParam(r *http.Request) database.ChatAutomation {
	automation, ok := r.Context().Value(chatAutomationParamContextKey{}).(database.ChatAutomation)
	if !ok {
		panic("developer error: chat automation param middleware not provided")
	}
	return automation
}

// ExtractChatAutomationParam grabs a chat automation from the
// "chatAutomation" URL parameter.
func ExtractChatAutomationParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			automationID, parsed := ParseUUIDParam(rw, r, "chatAutomation")
			if !parsed {
				return
			}

			automation, err := db.GetChatAutomationByID(ctx, automationID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching chat automation.",
					Detail:  err.Error(),
				})
				return
			}

			ctx = context.WithValue(ctx, chatAutomationParamContextKey{}, automation)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
