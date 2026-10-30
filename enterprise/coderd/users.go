package coderd

import (
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/codersdk"
)

const TimeFormatHHMM = "15:04"

func (api *API) autostopRequirementEnabledMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		feature, ok := api.Entitlements.Feature(codersdk.FeatureAdvancedTemplateScheduling)
		if !ok || !feature.Entitlement.Entitled() {
			httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{
				Message: "Advanced template scheduling (and user quiet hours schedule) is an Enterprise feature. Contact sales!",
			})
			return
		}
		if !feature.Enabled {
			httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{
				Message: "Advanced template scheduling (and user quiet hours schedule) is not enabled.",
			})
			return
		}

		next.ServeHTTP(rw, r)
	})
}

// @Summary Get user quiet hours schedule
// @Description Returns the daily quiet hours schedule that applies to a user, which is the deployment default until the user sets their own.
// @Description Check `user_set` to see whether the user set the schedule, and `user_can_set` to see whether the deployment lets users set their own.
// @Description Requires a Premium license.
// @ID get-user-quiet-hours-schedule
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param user path string true "User ID, username, or `me` for the authenticated user." format(uuid)
// @Success 200 {array} codersdk.UserQuietHoursScheduleResponse
// @Router /api/v2/users/{user}/quiet-hours [get]
func (api *API) userQuietHoursSchedule(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		user = httpmw.UserParam(r)
	)

	opts, err := (*api.UserQuietHoursScheduleStore.Load()).Get(ctx, api.Database, user.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if opts.Schedule == nil {
		httpapi.ResourceNotFound(rw)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserQuietHoursScheduleResponse{
		RawSchedule: opts.Schedule.String(),
		UserSet:     opts.UserSet,
		UserCanSet:  opts.UserCanSet,
		Time:        opts.Schedule.TimeParsed().Format(TimeFormatHHMM),
		Timezone:    opts.Schedule.Location().String(),
		Next:        opts.Schedule.Next(time.Now().In(opts.Schedule.Location())),
	})
}

// @Summary Update user quiet hours schedule
// @Description Sets the daily quiet hours schedule for a user, as a cron expression with a single time and an optional `CRON_TZ=` prefix, for example `CRON_TZ=UTC 0 0 * * *`.
// @Description Returns `403` when the deployment doesn't let users set their own schedule, which `--allow-custom-quiet-hours=false` controls.
// @Description You can update your own schedule, and updating another user's schedule requires permission to update that user's personal data, which the Owner and User Admin roles have.
// @Description Requires a Premium license.
// @ID update-user-quiet-hours-schedule
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param user path string true "User ID, username, or `me` for the authenticated user." format(uuid)
// @Param request body codersdk.UpdateUserQuietHoursScheduleRequest true "Quiet hours schedule to set."
// @Success 200 {array} codersdk.UserQuietHoursScheduleResponse
// @Router /api/v2/users/{user}/quiet-hours [put]
func (api *API) putUserQuietHoursSchedule(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		user              = httpmw.UserParam(r)
		params            codersdk.UpdateUserQuietHoursScheduleRequest
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

	opts, err := (*api.UserQuietHoursScheduleStore.Load()).Set(ctx, api.Database, user.ID, params.Schedule)
	if xerrors.Is(err, schedule.ErrUserCannotSetQuietHoursSchedule) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Users cannot set custom quiet hours schedule due to deployment configuration.",
		})
		return
	} else if err != nil {
		// TODO(@dean): some of these errors are related to bad syntax, so it
		// would be nice to 400 instead
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UserQuietHoursScheduleResponse{
		RawSchedule: opts.Schedule.String(),
		UserSet:     opts.UserSet,
		UserCanSet:  opts.UserCanSet,
		Time:        opts.Schedule.TimeParsed().Format(TimeFormatHHMM),
		Timezone:    opts.Schedule.Location().String(),
		Next:        opts.Schedule.Next(time.Now().In(opts.Schedule.Location())),
	})
}
