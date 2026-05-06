package coderd

import (
	"strings"
	"time"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

// templateMetaUpdate is the resolved set of values to apply for a
// PATCH /templates/{template} request. Any field on
// codersdk.UpdateTemplateMeta that is nil falls back to the existing
// template's value so that omitted request fields are not modified.
type templateMetaUpdate struct {
	name                                 string
	displayName                          string
	description                          string
	icon                                 string
	defaultTTLMillis                     int64
	activityBumpMillis                   int64
	failureTTLMillis                     int64
	timeTilDormantMillis                 int64
	timeTilDormantAutoDeleteMillis       int64
	allowUserAutostart                   bool
	allowUserAutostop                    bool
	allowUserCancelWorkspaceJobs         bool
	requireActiveVersion                 bool
	deprecationMessage                   string
	classicTemplateFlow                  bool
	disableModuleCache                   bool
	corsBehavior                         database.CorsBehavior
	autostopRequirementDaysOfWeekParsed  uint8
	autostartRequirementDaysOfWeekParsed uint8
	autostopRequirementWeeks             int64
	// disableEveryoneIntent is true only when the request explicitly asks
	// to disable the everyone-group access AND that group is currently
	// part of the template's GroupACL. It is used as a one-shot intent.
	disableEveryoneIntent bool
	// updateWorkspaceLastUsedAt and updateWorkspaceDormantAt are one-shot
	// intents that trigger side effects only when the request explicitly
	// sets the field to true. nil and false are no-ops.
	updateWorkspaceLastUsedAt bool
	updateWorkspaceDormantAt  bool
}

// resolveTemplateMetaUpdate produces a templateMetaUpdate populated with
// either the request value (when present) or the existing template's
// value (when the request field is nil).
//
// It also performs the parsing-only portion of validation for the
// autostop/autostart day-of-week bitmaps and CORS behavior. Any errors
// produced here are user-facing validation errors that the caller must
// surface as 400 Bad Request.
//
// Validation that depends on external interfaces (port sharing
// licensure) is intentionally left to the caller.
func resolveTemplateMetaUpdate(
	template database.Template,
	scheduleOpts schedule.TemplateScheduleOptions,
	req codersdk.UpdateTemplateMeta,
) (templateMetaUpdate, []codersdk.ValidationError) {
	var validErrs []codersdk.ValidationError

	out := templateMetaUpdate{
		name:                           ptr.NilToDefault(req.Name, template.Name),
		displayName:                    ptr.NilToDefault(req.DisplayName, template.DisplayName),
		description:                    ptr.NilToDefault(req.Description, template.Description),
		icon:                           ptr.NilToDefault(req.Icon, template.Icon),
		allowUserAutostart:             ptr.NilToDefault(req.AllowUserAutostart, template.AllowUserAutostart),
		allowUserAutostop:              ptr.NilToDefault(req.AllowUserAutostop, template.AllowUserAutostop),
		allowUserCancelWorkspaceJobs:   ptr.NilToDefault(req.AllowUserCancelWorkspaceJobs, template.AllowUserCancelWorkspaceJobs),
		requireActiveVersion:           ptr.NilToDefault(req.RequireActiveVersion, template.RequireActiveVersion),
		defaultTTLMillis:               ptr.NilToDefault(req.DefaultTTLMillis, time.Duration(template.DefaultTTL).Milliseconds()),
		activityBumpMillis:             ptr.NilToDefault(req.ActivityBumpMillis, time.Duration(template.ActivityBump).Milliseconds()),
		failureTTLMillis:               ptr.NilToDefault(req.FailureTTLMillis, time.Duration(template.FailureTTL).Milliseconds()),
		timeTilDormantMillis:           ptr.NilToDefault(req.TimeTilDormantMillis, time.Duration(template.TimeTilDormant).Milliseconds()),
		timeTilDormantAutoDeleteMillis: ptr.NilToDefault(req.TimeTilDormantAutoDeleteMillis, time.Duration(template.TimeTilDormantAutoDelete).Milliseconds()),
		deprecationMessage:             ptr.NilToDefault(req.DeprecationMessage, template.Deprecated),
		classicTemplateFlow:            ptr.NilToDefault(req.UseClassicParameterFlow, template.UseClassicParameterFlow),
		disableModuleCache:             ptr.NilToDefault(req.DisableModuleCache, template.DisableModuleCache),
		corsBehavior:                   template.CorsBehavior,
	}

	// Users should not be able to clear the template name. This is the only field
	// that treats a zero value as omitted.
	if out.name == "" {
		out.name = template.Name
	}

	// Resolve autostop requirement (defaults to the schedule store).
	autostopReq := req.AutostopRequirement
	if autostopReq == nil {
		autostopReq = &codersdk.TemplateAutostopRequirement{
			DaysOfWeek: codersdk.BitmapToWeekdays(scheduleOpts.AutostopRequirement.DaysOfWeek),
			Weeks:      scheduleOpts.AutostopRequirement.Weeks,
		}
	}
	if len(autostopReq.DaysOfWeek) > 0 {
		bitmap, err := codersdk.WeekdaysToBitmap(autostopReq.DaysOfWeek)
		if err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{
				Field:  "autostop_requirement.days_of_week",
				Detail: err.Error(),
			})
		} else {
			out.autostopRequirementDaysOfWeekParsed = bitmap
		}
	}
	out.autostopRequirementWeeks = autostopReq.Weeks
	if out.autostopRequirementWeeks == 0 {
		out.autostopRequirementWeeks = 1
	}

	// Resolve autostart requirement (defaults to the schedule store).
	autostartReq := req.AutostartRequirement
	if autostartReq == nil {
		autostartReq = &codersdk.TemplateAutostartRequirement{
			DaysOfWeek: codersdk.BitmapToWeekdays(scheduleOpts.AutostartRequirement.DaysOfWeek),
		}
	}
	if len(autostartReq.DaysOfWeek) > 0 {
		bitmap, err := codersdk.WeekdaysToBitmap(autostartReq.DaysOfWeek)
		if err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{
				Field:  "autostart_requirement.days_of_week",
				Detail: err.Error(),
			})
		} else {
			out.autostartRequirementDaysOfWeekParsed = bitmap
		}
	}

	// Resolve CORS behavior. An empty string is treated as "do not
	// change" because the existing UI-driven flow used to send empty
	// strings for unset values. A non-empty invalid value is a
	// validation error.
	if req.CORSBehavior != nil && *req.CORSBehavior != "" {
		val := database.CorsBehavior(*req.CORSBehavior)
		if !val.Valid() {
			validErrs = append(validErrs, codersdk.ValidationError{
				Field: "cors_behavior",
				Detail: "Invalid CORS behavior \"" + string(*req.CORSBehavior) +
					"\". Must be one of [" + strings.Join(slice.ToStrings(database.AllCorsBehaviorValues()), ", ") + "]",
			})
		} else {
			out.corsBehavior = val
		}
	}

	// disableEveryoneIntent is true only when the caller explicitly asks
	// for it AND the everyone group is currently in the template's
	// GroupACL. We compute this here so the no-op short-circuit in the
	// caller can correctly detect that an otherwise-empty request still
	// has an effect.
	if req.DisableEveryoneGroupAccess != nil && *req.DisableEveryoneGroupAccess {
		if _, ok := template.GroupACL[template.OrganizationID.String()]; ok {
			out.disableEveryoneIntent = true
		}
	}

	// One-shot intent flags. nil and false are both no-ops; true is a
	// trigger to run the side effect.
	if req.UpdateWorkspaceLastUsedAt != nil && *req.UpdateWorkspaceLastUsedAt {
		out.updateWorkspaceLastUsedAt = true
	}
	if req.UpdateWorkspaceDormantAt != nil && *req.UpdateWorkspaceDormantAt {
		out.updateWorkspaceDormantAt = true
	}

	return out, validErrs
}
