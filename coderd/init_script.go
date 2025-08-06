package coderd

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

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

	if os != "linux" && os != "darwin" && os != "windows" {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	if arch != "amd64" && arch != "arm64" && arch != "armv7" {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	script, exists := provisionersdk.AgentScriptEnv()[fmt.Sprintf("CODER_AGENT_SCRIPT_%s_%s", os, arch)]
	if !exists {
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	script = strings.ReplaceAll(script, "${ACCESS_URL}", api.AccessURL.String()+"/")
	script = strings.ReplaceAll(script, "${AUTH_TYPE}", "token")

	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write([]byte(script))
}
