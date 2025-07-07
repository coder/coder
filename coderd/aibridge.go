package coderd

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/aibridged"
	"github.com/coder/coder/v2/coderd/util/slice"
)

type rt struct {
	http.RoundTripper

	server *aibridged.Server
}

func (r *rt) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.RoundTripper.RoundTrip(req)

	if err != nil || resp.StatusCode == aibridged.ProxyErrCode {
		lastErr := r.server.BridgeErr()
		if lastErr != nil {
			return resp, lastErr
		}
	}

	return resp, err
}

func (api *API) bridgeAIRequest(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Something, somewhere is adding a duplicate header.
	// Haven't been able to track it down yet.
	// rw.Header().Del("Access-Control-Allow-Origin")

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
	rp.Transport = &rt{RoundTripper: http.DefaultTransport, server: server}
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		api.Logger.Error(ctx, "aibridge reverse proxy error", slog.Error(err))
	}
	http.StripPrefix("/api/v2/aibridge", rp).ServeHTTP(rw, r)
}
