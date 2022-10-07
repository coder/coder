package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
)

const (
	secretValue = "********"
)

func (api *API) deploymentSettings(rw http.ResponseWriter, r *http.Request) {
	df := *api.DeploymentFlags
	df.Oauth2GithubClientSecret.Value = secretValue
	df.OidcClientSecret.Value = secretValue
	df.PostgresURL.Value = secretValue

	httpapi.Write(r.Context(), rw, http.StatusOK, df)
}
