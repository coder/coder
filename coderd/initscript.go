package coderd

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

// @Summary Get agent init script
// @ID get-agent-init-script
// @Produce text/plain
// @Tags InitScript
// @Param os path string true "Operating system"
// @Param arch path string true "Architecture"
// @Success 200 "Success"
// @Router /init-script/{os}/{arch} [get]
func (api *API) initScript(rw http.ResponseWriter, r *http.Request) {
	os := strings.ToLower(chi.URLParam(r, "os"))
	arch := strings.ToLower(chi.URLParam(r, "arch"))

	script, exists := provisionersdk.AgentScriptEnv()[fmt.Sprintf("CODER_AGENT_SCRIPT_%s_%s", os, arch)]
	if !exists {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Unknown os/arch: %s/%s", os, arch),
		})
		return
	}
	script = strings.ReplaceAll(script, "${ACCESS_URL}", api.AccessURL.String()+"/")
	script = strings.ReplaceAll(script, "${AUTH_TYPE}", "token")

	scriptBytes := []byte(script)
	hash := sha256.Sum256(scriptBytes)
	rw.Header().Set("Content-Digest", fmt.Sprintf("sha256:%x", base64.StdEncoding.EncodeToString(hash[:])))
	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(scriptBytes)
}
