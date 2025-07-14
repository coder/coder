package coderd

import (
	"github.com/coder/coder/v2/coderd/httpmw"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) createUserSecret(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = httpmw.APIKey(r)

		req codersdk.CreateTemplateVersionRequest
	)

	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	//api.Database.GetUserByID()
}
