package coderd

import (
	"encoding/json"
	"net/http"

	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func (api *API) handleGetChecks(w http.ResponseWriter, _ *http.Request) {
	res := api.Checker.Results()
	if res == nil {
		httpapi.InternalServerError(w, xerrors.New("checks have not run yet, try again later"))
		return
	}
	b, err := json.Marshal(api.Checker.Results())
	if err != nil {
		httpapi.InternalServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (*API) handleCheckWebsocket(rw http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	_ = conn.Write(r.Context(), websocket.MessageText, []byte("ok"))
	_ = conn.Close(websocket.StatusNormalClosure, "all done")
}
