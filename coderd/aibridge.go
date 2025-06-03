package coderd

import (
	"net/http"
	"time"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/aibridged"
	"github.com/coder/coder/v2/coderd/util/slice"
)

func (api *API) bridgeOpenAIRequest(rw http.ResponseWriter, r *http.Request) {
	api.bridgeAIRequest(rw, r, aibridged.AIProviderOpenAI)
}

func (api *API) bridgeAnthropicRequest(rw http.ResponseWriter, r *http.Request) {
	api.bridgeAIRequest(rw, r, aibridged.AIProviderAnthropic)
}

func (api *API) bridgeAIRequest(rw http.ResponseWriter, r *http.Request, client any) {
	ctx := r.Context()

	if len(api.AIBridgeDaemons) == 0 {
		http.Error(rw, "no AI bridge daemons running", http.StatusInternalServerError)
		return
	}

	server, err := slice.PickRandom(api.AIBridgeDaemons)
	if err != nil {
		api.Logger.Error(ctx, "failed to pick random AI bridge server", slog.Error(err))
		http.Error(rw, "failed to select AI bridge", http.StatusInternalServerError)
		return
	}

	// TODO: use same context or new?
	c, err := api.CreateInMemoryOpenAIBridgeClient(ctx, server)
	if err != nil {
		api.Logger.Error(ctx, "failed to create OpenAI bridge", slog.Error(err))
		http.Error(rw, "failed to create OpenAI bridge", http.StatusInternalServerError)
		return
	}

	// TODO: don't create a new proxy on each request.
	proxy, err := aibridged.NewDRPCProxy(aibridged.NewOpenAIAdapter(c), aibridged.ProxyConfig{
		ReadTimeout: time.Second * 60, // TODO: read timeout.

	})

	if err != nil {
		api.Logger.Error(ctx, "failed to proxy HTTP request to AI bridge daemon", slog.Error(err))
		http.Error(rw, "failed to proxy HTTP request to AI bridge", http.StatusInternalServerError)
		return
	}

	http.StripPrefix("/api/v2/aibridge", proxy).ServeHTTP(rw, r)
}
