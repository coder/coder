package coderd

import (
	"net/http"

	"github.com/go-chi/render"

	"github.com/coder/coder/cli/buildinfo"
	"github.com/coder/coder/codersdk"
)

func (*api) buildInfo(rw http.ResponseWriter, r *http.Request) {
	render.JSON(rw, r, codersdk.BuildInfoResponse{
		Version: buildinfo.Version(),
	})
}
