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
	timeTilAutostopNotifyMillis          int64
	failureTTLMillis                     int64
	timeTilDormantMillis                 int64
	timeTilDormantAutoDeleteMillis       int64
	allowUserAutostart                   bool
	allowUserAutostop                    bool
	allowUserCancelWorkspaceJobs         bool
	requireActiveVersion                 bool
	deprecationMessage                   string
	useClassicTemplateFlow               bool
	disableModuleCache                   bool
	corsBehavior                         database.CorsBehavior
	autostopRequirementDaysOfWeekParsed  uint8
	autostartRequirementDaysOfWeekParsed uint8
	autostopRequirementWeeks             int64
	groupACL                             database.TemplateACL

	// updateWorkspaceLastUsedAtIntent and updateWorkspaceDormantAtIntent are one-shot
	// intents that trigger side effects only when the request explicitly
	// sets the field to true. nil and false are no-ops.
	updateWorkspaceLastUsedAtIntent bool
	updateWorkspaceDormantAtIntent  bool
}

// resolveTemplateMetaUpdate produces a templateMetaUpdate populated with
// either the request value (when present) or the existing template's
// value (when the request field is nil).
//
// This function validates shape, not contents: it parses the
// autostop/autostart day-of-week strings into bitmaps and ensures any
// non-empty CORS behavior is a recognized enum. Errors it returns are
// user-facing validation errors the caller must surface as 400 Bad
// Request.
//
// Range and content checks (e.g. activityBumpMillis >= 0,
// failureTTLMillis >= 1 minute, max port share level) and validation
// that depends on external interfaces (such as port-sharing licensure)
// are the caller's responsibility.
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
		defaultTTLMillis:               ptr.NilToDefault(req.DefaultTTLMillis, time.Duration(template.DefaultTTL).Milliseconds()),
		activityBumpMillis:             ptr.NilToDefault(req.ActivityBumpMillis, time.Duration(template.ActivityBump).Milliseconds()),
		timeTilAutostopNotifyMillis:    ptr.NilToDefault(req.TimeTilAutostopNotifyMillis, time.Duration(template.TimeTilAutostopNotify).Milliseconds()),
		failureTTLMillis:               ptr.NilToDefault(req.FailureTTLMillis, time.Duration(template.FailureTTL).Milliseconds()),
		timeTilDormantMillis:           ptr.NilToDefault(req.TimeTilDormantMillis, time.Duration(template.TimeTilDormant).Milliseconds()),
		timeTilDormantAutoDeleteMillis: ptr.NilToDefault(req.TimeTilDormantAutoDeleteMillis, time.Duration(template.TimeTilDormantAutoDelete).Milliseconds()),
		allowUserAutostart:             ptr.NilToDefault(req.AllowUserAutostart, template.AllowUserAutostart),
		allowUserAutostop:              ptr.NilToDefault(req.AllowUserAutostop, template.AllowUserAutostop),
		allowUserCancelWorkspaceJobs:   ptr.NilToDefault(req.AllowUserCancelWorkspaceJobs, template.AllowUserCancelWorkspaceJobs),
		requireActiveVersion:           ptr.NilToDefault(req.RequireActiveVersion, template.RequireActiveVersion),
		deprecationMessage:             ptr.NilToDefault(req.DeprecationMessage, template.Deprecated),
		useClassicTemplateFlow:         ptr.NilToDefault(req.UseClassicParameterFlow, template.UseClassicParameterFlow),
		disableModuleCache:             ptr.NilToDefault(req.DisableModuleCache, template.DisableModuleCache),
		groupACL:                       template.GroupACL,

		// Default to the original values
		corsBehavior:                         template.CorsBehavior,
		autostopRequirementDaysOfWeekParsed:  scheduleOpts.AutostopRequirement.DaysOfWeek,
		autostopRequirementWeeks:             scheduleOpts.AutostopRequirement.Weeks,
		autostartRequirementDaysOfWeekParsed: scheduleOpts.AutostartRequirement.DaysOfWeek,
		updateWorkspaceLastUsedAtIntent:      false,
		updateWorkspaceDormantAtIntent:       false,
	}

	// Users should not be able to clear the template name. This is the only field
	// that treats a zero value as omitted.
	if out.name == "" {
		out.name = template.Name
	}

	// Override autostop if provided is non-nil
	if req.AutostopRequirement != nil {
		bitmap, err := codersdk.WeekdaysToBitmap(req.AutostopRequirement.DaysOfWeek)
		if err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{
				Field:  "autostop_requirement.days_of_week",
				Detail: err.Error(),
			})
		} else {
			out.autostopRequirementDaysOfWeekParsed = bitmap
			out.autostopRequirementWeeks = req.AutostopRequirement.Weeks
		}

		// Always force <= 0 -> 1
		if out.autostopRequirementWeeks <= 0 {
			out.autostopRequirementWeeks = defaultRequirementWeeks
		}
	}

	// Override autostart if provided is non-nil
	if req.AutostartRequirement != nil {
		bitmap, err := codersdk.WeekdaysToBitmap(req.AutostartRequirement.DaysOfWeek)
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

	if req.DisableEveryoneGroupAccess != nil && *req.DisableEveryoneGroupAccess {
		// Remove the "everyone" group from the template. If this is set to false, the
		// user needs to explicitly add the "everyone" group back to the ACL via the
		// group ACL endpoints, so we don't treat false as a no-op.
		delete(out.groupACL, template.OrganizationID.String())
	}

	// One-shot intent flags. nil and false are both no-ops; true is a
	// trigger to run the side effect.
	if req.UpdateWorkspaceLastUsedAt != nil && *req.UpdateWorkspaceLastUsedAt {
		out.updateWorkspaceLastUsedAtIntent = true
	}
	if req.UpdateWorkspaceDormantAt != nil && *req.UpdateWorkspaceDormantAt {
		out.updateWorkspaceDormantAtIntent = true
	}

	return out, validErrs
}
