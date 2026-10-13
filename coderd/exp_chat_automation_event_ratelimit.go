package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

// chatAutomationEventLookup is the automation a webhook event request
// names, read once before the rate limiter so the limiter and the handler
// share the read.
type chatAutomationEventLookup struct {
	automation database.ChatAutomation
	// found is false for ids that do not parse, do not exist, or name an
	// automation that is not a webhook.
	found bool
}

type chatAutomationEventLookupKey struct{}

// lookupChatAutomationEvent reads the automation that the {automation}
// path parameter names, as chatd: webhook callers have no Coder identity.
func (api *API) lookupChatAutomationEvent(ctx context.Context, r *http.Request) (chatAutomationEventLookup, error) {
	id, err := uuid.Parse(chi.URLParam(r, "automation"))
	if err != nil {
		return chatAutomationEventLookup{}, nil
	}
	//nolint:gocritic // Webhook callers authenticate only with the automation secret, which the handler checks next.
	automation, err := api.Database.GetChatAutomationByID(dbauthz.AsChatd(ctx), id)
	if errors.Is(err, sql.ErrNoRows) {
		return chatAutomationEventLookup{}, nil
	}
	if err != nil {
		return chatAutomationEventLookup{}, err
	}
	return chatAutomationEventLookup{automation: automation, found: automation.Kind == database.ChatAutomationKindWebhook}, nil
}

// chatAutomationEventRateLimit limits webhook event requests per
// automation and caller IP, count per window.
//
// The automation is read before the limiter, so the bucket can name it:
// one sender can feed several automations without one starving the
// others. Requests for an automation that does not exist share one
// bucket per IP, so random ids get no fresh bucket. The read is one
// primary-key lookup, which the handler then reuses, so a request costs
// the same read as before; a request refused by the limiter still pays
// it.
func (api *API) chatAutomationEventRateLimit(count int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limited := next
		// -1 is no rate limit.
		if count > 0 {
			limited = httprate.Limit(count, window,
				httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
					lookup, _ := r.Context().Value(chatAutomationEventLookupKey{}).(chatAutomationEventLookup)
					if !lookup.found {
						return "chat-automation-events:unknown", nil
					}
					return "chat-automation-events:" + lookup.automation.ID.String(), nil
				}, httprate.KeyByIP),
				httprate.WithLimitHandler(func(rw http.ResponseWriter, r *http.Request) {
					if api.chatDaemon != nil {
						api.chatDaemon.RecordAutomationWebhookRejected(chatd.AutomationWebhookRejectedRateLimited)
					}
					httpapi.Write(r.Context(), rw, http.StatusTooManyRequests, codersdk.Response{
						Message: fmt.Sprintf("You've been rate limited for sending more than %v requests in %v.", count, window),
					})
				}),
			)(next)
		}
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			lookup, err := api.lookupChatAutomationEvent(r.Context(), r)
			if err != nil {
				httpapi.InternalServerError(rw, err)
				return
			}
			limited.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), chatAutomationEventLookupKey{}, lookup)))
		})
	}
}
