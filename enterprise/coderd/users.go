package coderd

import (
	"net/http"
	"time"

	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

// @Summary Get user maintenance schedule
// @ID get-user-maintenance-schedule
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param user path string true "User ID" format(uuid)
// @Success 200 {array} codersdk.UserMaintenanceScheduleResponse
// @Router /users/{user}/maintenance-schedule [get]
func (api *API) userMaintenanceSchedule(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		user = httpmw.UserParam(r)
	)

	// TODO: Double query here cuz of the user param
	opts, err := (*api.UserMaintenanceScheduleStore.Load()).GetUserMaintenanceScheduleOptions(ctx, api.Database, user.ID)
	if err != nil {
		// TODO: some of these errors are related to bad syntax, would be nice
		// to 400
		httpapi.InternalServerError(rw, err)
		return
	}
	if opts.Schedule == nil {
		httpapi.ResourceNotFound(rw)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserMaintenanceScheduleResponse{
		RawSchedule: opts.Schedule.String(),
		UserSet:     opts.UserSet,
		Time:        opts.Schedule.Time(),
		Timezone:    opts.Schedule.Location().String(),
		Duration:    opts.Duration,
		Next:        opts.Schedule.Next(time.Now()),
	})
}

// @Summary Update user maintenance schedule
// @ID update-user-maintenance-schedule
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param user path string true "User ID" format(uuid)
// @Param request body codersdk.UpdateUserMaintenanceScheduleRequest true "Update schedule request"
// @Success 200 {array} codersdk.UserMaintenanceScheduleResponse
// @Router /users/{user}/maintenance-schedule [put]
func (api *API) putUserMaintenanceSchedule(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		params            codersdk.UpdateUserMaintenanceScheduleRequest
		aReq, commitAudit = audit.InitRequest[database.User](rw, &audit.RequestParams{
			Audit:   api.Auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()
	aReq.Old = user

	if !httpapi.Read(ctx, rw, r, &params) {
		return
	}

	opts, err := (*api.UserMaintenanceScheduleStore.Load()).SetUserMaintenanceScheduleOptions(ctx, api.Database, user.ID, params.Schedule)
	if err != nil {
		// TODO: some of these errors are related to bad syntax, would be nice
		// to 400
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserMaintenanceScheduleResponse{
		RawSchedule: opts.Schedule.String(),
		UserSet:     opts.UserSet,
		Time:        opts.Schedule.Time(),
		Timezone:    opts.Schedule.Location().String(),
		Duration:    opts.Duration,
		Next:        opts.Schedule.Next(time.Now()),
	})
}
