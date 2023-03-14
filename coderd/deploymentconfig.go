package coderd

import (
	"math"
	"net/http"
	"time"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// @Summary Get deployment config
// @ID get-deployment-config
// @Security CoderSessionToken
// @Produce json
// @Tags General
// @Success 200 {object} codersdk.DeploymentConfig
// @Router /config/deployment [get]
func (api *API) deploymentValues(rw http.ResponseWriter, r *http.Request) {
	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceDeploymentValues) {
		httpapi.Forbidden(rw)
		return
	}

	values, err := api.DeploymentValues.WithoutSecrets()
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(
		r.Context(), rw, http.StatusOK,
		codersdk.DeploymentConfig{
			Values:  values,
			Options: values.Options(),
		},
	)
}

// @Summary Get token config
// @ID get-token-config
// @Security CoderSessionToken
// @Produce json
// @Tags General
// @Success 200 {object} codersdk.TokenConfig
// @Router /config/tokenconfig [get]
func (api *API) tokenConfig(rw http.ResponseWriter, r *http.Request) {
	values, err := api.DeploymentValues.WithoutSecrets()
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	var maxTokenLifetime time.Duration
	// if --max-token-lifetime is unset (default value is math.MaxInt64)
	// send back a falsy value
	if values.MaxTokenLifetime.Value() == time.Duration(math.MaxInt64) {
		maxTokenLifetime = 0
	} else {
		maxTokenLifetime = values.MaxTokenLifetime.Value()
	}

	httpapi.Write(
		r.Context(), rw, http.StatusOK,
		codersdk.TokenConfig{
			MaxTokenLifetime: maxTokenLifetime,
		},
	)
}
