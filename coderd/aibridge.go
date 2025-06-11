package coderd

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/util/slice"
)

func (api *API) bridgeAIRequest(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Something, somewhere is adding a duplicate header.
	// Haven't been able to track it down yet.
	rw.Header().Del("Access-Control-Allow-Origin")

	if len(api.AIBridgeDaemons) == 0 {
		http.Error(rw, "no AI bridge daemons running", http.StatusInternalServerError)
		return
	}

	// Random loadbalancing.
	// TODO: introduce better strategy.
	server, err := slice.PickRandom(api.AIBridgeDaemons)
	if err != nil {
		api.Logger.Error(ctx, "failed to pick random AI bridge server", slog.Error(err))
		http.Error(rw, "failed to select AI bridge", http.StatusInternalServerError)
		return
	}

	u, err := url.Parse(fmt.Sprintf("http://%s", server.BridgeAddr())) // TODO: TLS.
	if err != nil {
		api.Logger.Error(ctx, "failed to parse bridge address", slog.Error(err))
		http.Error(rw, "failed to parse bridge address", http.StatusInternalServerError)
	}

	rp := httputil.NewSingleHostReverseProxy(u)
	http.StripPrefix("/api/v2/aibridge", rp).ServeHTTP(rw, r)
}
