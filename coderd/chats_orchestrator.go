package coderd

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// orchestratorChatTitle is the fixed title of every orchestrator chat.
const orchestratorChatTitle = "Orchestrator"

// orchestratorChatEnabled mirrors httpmw.RequireExperimentWithDevBypass so
// dev builds can exercise the orchestrator without flipping the experiment.
func (api *API) orchestratorChatEnabled() bool {
	return buildinfo.IsDev() || api.Experiments.Enabled(codersdk.ExperimentChatOrchestrator)
}

// resolveOrchestratorChatAlias rewrites the literal "orchestrator" chat path
// segment to the caller's deterministic orchestrator chat ID so every
// /chats/{chat} route addresses the orchestrator without dedicated handlers.
// The ID is derived from the authenticated user, so it can only ever resolve
// to the caller's own orchestrator; ExtractChatParam still returns 404 until
// that chat has been created.
func (api *API) resolveOrchestratorChatAlias(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if chi.URLParam(r, "chat") == codersdk.OrchestratorChatAlias && api.orchestratorChatEnabled() {
			chatID := codersdk.OrchestratorChatID(httpmw.APIKey(r).UserID).String()
			rctx := chi.RouteContext(r.Context())
			for i, key := range rctx.URLParams.Keys {
				if key == "chat" {
					rctx.URLParams.Values[i] = chatID
				}
			}
		}
		next.ServeHTTP(rw, r)
	})
}

// isOrchestratorChat reports whether the chat is the owner's orchestrator.
func isOrchestratorChat(chat database.Chat) bool {
	return chat.Mode.Valid && chat.Mode.ChatMode == database.ChatModeOrchestrator
}
