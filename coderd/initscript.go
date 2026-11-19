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
// @Description Returns the script that downloads the workspace agent binary from the deployment and starts it, for the given operating system and architecture.
// @Description The script has the deployment's access URL filled in and token authentication selected, and the Coder Terraform provider builds the `init_script` of a `coder_agent` from the same script.
// @Description Doesn't require a session token, and returns `400` for an operating system and architecture pair that Coder doesn't support.
// @ID get-agent-init-script
// @Produce text/plain
// @Tags InitScript
// @Param os path string true "Operating system of the machine that runs the agent, one of `linux`, `darwin`, or `windows`. The match ignores case."
// @Param arch path string true "CPU architecture of the machine that runs the agent, one of `amd64` or `arm64`, or `armv7` on `linux`. The match ignores case."
// @Success 200 "Success"
// @Router /api/v2/init-script/{os}/{arch} [get]
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
