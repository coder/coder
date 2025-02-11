package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get provisioner daemons
// @ID get-provisioner-daemons
// @Security CoderSessionToken
// @Produce json
// @Tags Provisioning
// @Param organization path string true "Organization ID" format(uuid)
// @Param tags query object false "Provisioner tags to filter by (JSON of the form {'tag1':'value1','tag2':'value2'})"
// @Success 200 {array} codersdk.ProvisionerDaemon
// @Router /organizations/{organization}/provisionerdaemons [get]
func (api *API) provisionerDaemons(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx      = r.Context()
		org      = httpmw.OrganizationParam(r)
		tagParam = r.URL.Query().Get("tags")
		tags     = database.StringMap{}
		err      = tags.Scan([]byte(tagParam))
	)

	if tagParam != "" && err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid tags query parameter",
			Detail:  err.Error(),
		})
		return
	}

	daemons, err := api.Database.GetProvisionerDaemonsWithStatusByOrganization(
		ctx,
		database.GetProvisionerDaemonsWithStatusByOrganizationParams{
			OrganizationID:  org.ID,
			StaleIntervalMS: provisionerdserver.StaleInterval.Milliseconds(),
			Tags:            tags,
		},
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner daemons.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.List(daemons, func(dbDaemon database.GetProvisionerDaemonsWithStatusByOrganizationRow) codersdk.ProvisionerDaemon {
		pd := db2sdk.ProvisionerDaemon(dbDaemon.ProvisionerDaemon)
		var currentJob, previousJob *codersdk.ProvisionerDaemonJob
		if dbDaemon.CurrentJobID.Valid {
			currentJob = &codersdk.ProvisionerDaemonJob{
				ID:                  dbDaemon.CurrentJobID.UUID,
				Status:              codersdk.ProvisionerJobStatus(dbDaemon.CurrentJobStatus.ProvisionerJobStatus),
				TemplateName:        dbDaemon.CurrentJobTemplateName.String,
				TemplateIcon:        dbDaemon.CurrentJobTemplateIcon.String,
				TemplateDisplayName: dbDaemon.CurrentJobTemplateDisplayName.String,
			}
		}
		if dbDaemon.PreviousJobID.Valid {
			previousJob = &codersdk.ProvisionerDaemonJob{
				ID:     dbDaemon.PreviousJobID.UUID,
				Status: codersdk.ProvisionerJobStatus(dbDaemon.PreviousJobStatus.ProvisionerJobStatus),
			}
		}

		// Add optional fields.
		pd.KeyName = &dbDaemon.KeyName
		pd.Status = ptr.Ref(codersdk.ProvisionerDaemonStatus(dbDaemon.Status))
		pd.CurrentJob = currentJob
		pd.PreviousJob = previousJob

		return pd
	}))
}
