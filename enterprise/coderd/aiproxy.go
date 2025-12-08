package coderd

import (
	"encoding/pem"
	"net/http"

	"github.com/elazarl/goproxy"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get AI proxy CA certificate
// @ID get-ai-proxy-ca-cert
// @Security CoderSessionToken
// @Produce application/x-pem-file
// @Tags Enterprise
// @Success 200 {file} binary "PEM-encoded CA certificate"
// @Router /aiproxy/ca-cert [get]
func (api *API) aiproxyCACert(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	ca := goproxy.GoproxyCa
	if len(ca.Certificate) == 0 {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "AI proxy CA certificate not configured",
		})
		return
	}

	pemBlock := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: ca.Certificate[0],
	}

	rw.Header().Set("Content-Type", "application/x-pem-file")
	rw.Header().Set("Content-Disposition", "attachment; filename=aiproxy-ca.crt")
	rw.WriteHeader(http.StatusOK)

	if err := pem.Encode(rw, pemBlock); err != nil {
		api.Logger.Error(ctx, "failed to encode CA certificate")
	}
}
