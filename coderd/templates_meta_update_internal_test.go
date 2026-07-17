package coderd

import (
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// baselineTemplate returns a database.Template populated with non-default
// values for every field that resolveTemplateMetaUpdate reads. Non-default
// values let single-field tests detect when a field is being silently
// overwritten with a zero value.
func baselineTemplate() database.Template {
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	return database.Template{
		ID:                            uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		OrganizationID:                orgID,
		Name:                          "baseline",
		DisplayName:                   "Baseline Template",
		Description:                   "An existing description.",
		Icon:                          "/baseline.svg",
		AllowUserAutostart:            false,
		AllowUserAutostop:             false,
		AllowUserCancelWorkspaceJobs:  false,
		RequireActiveVersion:          true,
		DefaultTTL:                    int64(60 * 60 * 1000 * 1000 * 1000),  // 1 hour in ns
		ActivityBump:                  int64(30 * 60 * 1000 * 1000 * 1000),  // 30 minutes in ns
		TimeTilAutostopNotify:         int64(10 * 60 * 1000 * 1000 * 1000),  // 10 minutes in ns
		FailureTTL:                    int64(120 * 60 * 1000 * 1000 * 1000), // 2 hours in ns
		TimeTilDormant:                int64(240 * 60 * 1000 * 1000 * 1000), // 4 hours in ns
		TimeTilDormantAutoDelete:      int64(480 * 60 * 1000 * 1000 * 1000), // 8 hours in ns
		AutostopRequirementDaysOfWeek: 0b0000001,                            // Monday
		AutostopRequirementWeeks:      2,
		AutostartBlockDaysOfWeek:      0b1000000,    // Sunday
		Deprecated:                    "deprecated", // non-empty so the conversion is observable
		MaxPortSharingLevel:           database.AppSharingLevelOrganization,
		UseClassicParameterFlow:       true,
		CorsBehavior:                  database.CorsBehaviorPassthru,
		DisableModuleCache:            true,
		GroupACL: database.TemplateACL{
			orgID.String(): {"read"},
		},
	}
}

// baselineScheduleOpts returns schedule options matching the baseline
// template above, so that nil request fields resolve to these values.
func baselineScheduleOpts() schedule.TemplateScheduleOptions {
	return schedule.TemplateScheduleOptions{
		AutostopRequirement: schedule.TemplateAutostopRequirement{
			DaysOfWeek: 0b0000001,
			Weeks:      2,
		},
		AutostartRequirement: schedule.TemplateAutostartRequirement{
			DaysOfWeek: 0b1000000,
		},
	}
}

// baselineResolved returns the templateMetaUpdate that resolveTemplateMetaUpdate
// produces for an empty request against baselineTemplate / baselineScheduleOpts.
func baselineResolved() templateMetaUpdate {
	tpl := baselineTemplate()
	return templateMetaUpdate{
		name:                                 tpl.Name,
		displayName:                          tpl.DisplayName,
		description:                          tpl.Description,
		icon:                                 tpl.Icon,
		defaultTTLMillis:                     tpl.DefaultTTL / 1e6,
		activityBumpMillis:                   tpl.ActivityBump / 1e6,
		timeTilAutostopNotifyMillis:          tpl.TimeTilAutostopNotify / 1e6,
		failureTTLMillis:                     tpl.FailureTTL / 1e6,
		timeTilDormantMillis:                 tpl.TimeTilDormant / 1e6,
		timeTilDormantAutoDeleteMillis:       tpl.TimeTilDormantAutoDelete / 1e6,
		allowUserAutostart:                   tpl.AllowUserAutostart,
		allowUserAutostop:                    tpl.AllowUserAutostop,
		allowUserCancelWorkspaceJobs:         tpl.AllowUserCancelWorkspaceJobs,
		requireActiveVersion:                 tpl.RequireActiveVersion,
		deprecationMessage:                   tpl.Deprecated,
		useClassicTemplateFlow:               tpl.UseClassicParameterFlow,
		disableModuleCache:                   tpl.DisableModuleCache,
		corsBehavior:                         tpl.CorsBehavior,
		autostopRequirementDaysOfWeekParsed:  0b0000001,
		autostartRequirementDaysOfWeekParsed: 0b1000000,
		autostopRequirementWeeks:             tpl.AutostopRequirementWeeks,
		groupACL:                             tpl.GroupACL,
	}
}

func TestResolveTemplateMetaUpdate(t *testing.T) {
	t.Parallel()

	type expected struct {
		// override is applied to baselineResolved to produce the expected
		// templateMetaUpdate. Allows each case to express only its delta.
		override func(*templateMetaUpdate)
		base     func(template *database.Template)
		// validErrFields, if non-empty, asserts the resolver produced a
		// validation error for each named field.
		validErrFields []string
	}

	tests := []struct {
		name     string
		req      codersdk.UpdateTemplateMeta
		expected expected
	}{
		// Sanity check: an empty PATCH preserves every field.
		{
			name:     "EmptyRequestPreservesEverything",
			req:      codersdk.UpdateTemplateMeta{},
			expected: expected{override: func(*templateMetaUpdate) {}},
		},

		// One case per pointer field: each case sends only that field
		// and asserts only that field changed in the resolved struct.
		{
			name: "Name",
			req:  codersdk.UpdateTemplateMeta{Name: ptr.Ref("renamed")},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.name = "renamed"
			}},
		},
		{
			name: "NameEmptyStringFallsBackToCurrent",
			req:  codersdk.UpdateTemplateMeta{Name: ptr.Ref("")},
			// Empty string is treated as "do not clear" because the UI
			// disallows clearing the name. Resolver must keep the
			// existing name.
			// This is a unique case to just the `name` field.
			expected: expected{override: func(*templateMetaUpdate) {}},
		},
		{
			name: "DisplayName",
			req:  codersdk.UpdateTemplateMeta{DisplayName: ptr.Ref("Renamed")},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.displayName = "Renamed"
			}},
		},
		{
			name: "Description",
			req:  codersdk.UpdateTemplateMeta{Description: ptr.Ref("New description")},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.description = "New description"
			}},
		},
		{
			name: "Icon",
			req:  codersdk.UpdateTemplateMeta{Icon: ptr.Ref("/new.svg")},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.icon = "/new.svg"
			}},
		},
		{
			name: "DefaultTTLMillis",
			req:  codersdk.UpdateTemplateMeta{DefaultTTLMillis: ptr.Ref(int64(7200_000))},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.defaultTTLMillis = 7200_000
			}},
		},
		{
			name: "DefaultTTLMillisZeroExplicit",
			req:  codersdk.UpdateTemplateMeta{DefaultTTLMillis: ptr.Ref(int64(0))},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.defaultTTLMillis = 0
			}},
		},
		{
			name: "ActivityBumpMillis",
			req:  codersdk.UpdateTemplateMeta{ActivityBumpMillis: ptr.Ref(int64(900_000))},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.activityBumpMillis = 900_000
			}},
		},
		{
			name: "TimeTilAutostopNotifyMillis",
			req:  codersdk.UpdateTemplateMeta{TimeTilAutostopNotifyMillis: ptr.Ref(int64(300_000))},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.timeTilAutostopNotifyMillis = 300_000
			}},
		},
		{
			name: "TimeTilAutostopNotifyMillisZeroExplicit",
			req:  codersdk.UpdateTemplateMeta{TimeTilAutostopNotifyMillis: ptr.Ref(int64(0))},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.timeTilAutostopNotifyMillis = 0
			}},
		},
		{
			name: "AllowUserAutostart",
			req:  codersdk.UpdateTemplateMeta{AllowUserAutostart: ptr.Ref(true)},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.allowUserAutostart = true
			}},
		},
		{
			name: "AllowUserAutostop",
			req:  codersdk.UpdateTemplateMeta{AllowUserAutostop: ptr.Ref(true)},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.allowUserAutostop = true
			}},
		},
		{
			name: "AllowUserAutostop/true",
			req:  codersdk.UpdateTemplateMeta{AllowUserAutostop: ptr.Ref(false)},
			expected: expected{
				base: func(update *database.Template) {
					update.AllowUserAutostop = true
				},
				override: func(r *templateMetaUpdate) {
					r.allowUserAutostop = false
				},
			},
		},
		{
			name: "AllowUserCancelWorkspaceJobs",
			req:  codersdk.UpdateTemplateMeta{AllowUserCancelWorkspaceJobs: ptr.Ref(true)},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.allowUserCancelWorkspaceJobs = true
			}},
		},
		{
			name: "FailureTTLMillis",
			req:  codersdk.UpdateTemplateMeta{FailureTTLMillis: ptr.Ref(int64(3_600_000))},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.failureTTLMillis = 3_600_000
			}},
		},
		{
			name: "TimeTilDormantMillis",
			req:  codersdk.UpdateTemplateMeta{TimeTilDormantMillis: ptr.Ref(int64(7_200_000))},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.timeTilDormantMillis = 7_200_000
			}},
		},
		{
			name: "TimeTilDormantAutoDeleteMillis",
			req:  codersdk.UpdateTemplateMeta{TimeTilDormantAutoDeleteMillis: ptr.Ref(int64(14_400_000))},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.timeTilDormantAutoDeleteMillis = 14_400_000
			}},
		},
		{
			name: "RequireActiveVersion",
			req:  codersdk.UpdateTemplateMeta{RequireActiveVersion: ptr.Ref(false)},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.requireActiveVersion = false
			}},
		},
		{
			name: "DeprecationMessage",
			req:  codersdk.UpdateTemplateMeta{DeprecationMessage: ptr.Ref("now deprecated")},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.deprecationMessage = "now deprecated"
			}},
		},
		{
			name: "DeprecationMessageEmptyStringClears",
			req:  codersdk.UpdateTemplateMeta{DeprecationMessage: ptr.Ref("")},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.deprecationMessage = ""
			}},
		},
		{
			name: "UseClassicParameterFlow",
			req:  codersdk.UpdateTemplateMeta{UseClassicParameterFlow: ptr.Ref(false)},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.useClassicTemplateFlow = false
			}},
		},
		{
			name: "DisableModuleCache",
			req:  codersdk.UpdateTemplateMeta{DisableModuleCache: ptr.Ref(false)},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.disableModuleCache = false
			}},
		},

		// CORS behavior.
		{
			name: "CORSBehaviorChange",
			req: codersdk.UpdateTemplateMeta{
				CORSBehavior: ptr.Ref(codersdk.CORSBehavior(database.CorsBehaviorSimple)),
			},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.corsBehavior = database.CorsBehaviorSimple
			}},
		},
		{
			name: "CORSBehaviorEmptyStringPreserves",
			req: codersdk.UpdateTemplateMeta{
				CORSBehavior: ptr.Ref(codersdk.CORSBehavior("")),
			},
			// Empty string is treated as "do not change" for backwards
			// compatibility with older clients that always send the
			// field.
			expected: expected{override: func(*templateMetaUpdate) {}},
		},
		{
			name: "CORSBehaviorInvalid",
			req: codersdk.UpdateTemplateMeta{
				CORSBehavior: ptr.Ref(codersdk.CORSBehavior("not-a-real-value")),
			},
			expected: expected{
				// Invalid value: keep current and surface a validation error.
				override:       func(*templateMetaUpdate) {},
				validErrFields: []string{"cors_behavior"},
			},
		},

		// Autostop / autostart requirement bitmaps.
		{
			name: "AutostopRequirementChange",
			req: codersdk.UpdateTemplateMeta{
				AutostopRequirement: &codersdk.TemplateAutostopRequirement{
					DaysOfWeek: []string{"friday"},
					Weeks:      4,
				},
			},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.autostopRequirementDaysOfWeekParsed = 0b0010000
				r.autostopRequirementWeeks = 4
			}},
		},
		{
			name: "AutostopRequirementWeeksZeroNormalizesToOne",
			req: codersdk.UpdateTemplateMeta{
				AutostopRequirement: &codersdk.TemplateAutostopRequirement{
					DaysOfWeek: []string{"monday"},
					Weeks:      0,
				},
			},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.autostopRequirementDaysOfWeekParsed = 0b0000001
				r.autostopRequirementWeeks = 1
			}},
		},
		{
			name: "AutostopRequirementInvalidDay",
			req: codersdk.UpdateTemplateMeta{
				AutostopRequirement: &codersdk.TemplateAutostopRequirement{
					DaysOfWeek: []string{"funday"},
					Weeks:      1,
				},
			},
			expected: expected{
				override: func(r *templateMetaUpdate) {
					r.autostopRequirementDaysOfWeekParsed = 1
					r.autostopRequirementWeeks = 2
				},
				validErrFields: []string{"autostop_requirement.days_of_week"},
			},
		},
		{
			name: "AutostartRequirementChange",
			req: codersdk.UpdateTemplateMeta{
				AutostartRequirement: &codersdk.TemplateAutostartRequirement{
					DaysOfWeek: []string{"saturday"},
				},
			},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.autostartRequirementDaysOfWeekParsed = 0b0100000
			}},
		},
		{
			name: "AutostartRequirementInvalidDay",
			req: codersdk.UpdateTemplateMeta{
				AutostartRequirement: &codersdk.TemplateAutostartRequirement{
					DaysOfWeek: []string{"funday"},
				},
			},
			expected: expected{
				override: func(r *templateMetaUpdate) {
					r.autostartRequirementDaysOfWeekParsed = 64
				},
				validErrFields: []string{"autostart_requirement.days_of_week"},
			},
		},

		// One-shot intent flags. nil and false should both result in
		// the corresponding *Intent field being false; only true triggers it.
		{
			name: "DisableEveryoneGroupAccessFalseIsNoop",
			req:  codersdk.UpdateTemplateMeta{DisableEveryoneGroupAccess: ptr.Ref(false)},
			expected: expected{override: func(*templateMetaUpdate) {
				// disableEveryoneIntent stays false.
			}},
		},
		{
			name: "DisableEveryoneGroupAccessTrueWithMembership",
			req:  codersdk.UpdateTemplateMeta{DisableEveryoneGroupAccess: ptr.Ref(true)},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.groupACL = database.TemplateACL{}
			}},
		},
		{
			name:     "UpdateWorkspaceLastUsedAtFalseIsNoop",
			req:      codersdk.UpdateTemplateMeta{UpdateWorkspaceLastUsedAt: ptr.Ref(false)},
			expected: expected{override: func(*templateMetaUpdate) {}},
		},
		{
			name: "UpdateWorkspaceLastUsedAtTrue",
			req:  codersdk.UpdateTemplateMeta{UpdateWorkspaceLastUsedAt: ptr.Ref(true)},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.updateWorkspaceLastUsedAtIntent = true
			}},
		},
		{
			name:     "UpdateWorkspaceDormantAtFalseIsNoop",
			req:      codersdk.UpdateTemplateMeta{UpdateWorkspaceDormantAt: ptr.Ref(false)},
			expected: expected{override: func(*templateMetaUpdate) {}},
		},
		{
			name: "UpdateWorkspaceDormantAtTrue",
			req:  codersdk.UpdateTemplateMeta{UpdateWorkspaceDormantAt: ptr.Ref(true)},
			expected: expected{override: func(r *templateMetaUpdate) {
				r.updateWorkspaceDormantAtIntent = true
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tpl := baselineTemplate()
			if tc.expected.base != nil {
				tc.expected.base(&tpl)
			}
			schedOpts := baselineScheduleOpts()
			got, validErrs := resolveTemplateMetaUpdate(tpl, schedOpts, tc.req)

			want := baselineResolved()
			tc.expected.override(&want)

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("resolved mismatch\ngot:  %+v\nwant: %+v", got, want)
			}

			if len(validErrs) != len(tc.expected.validErrFields) {
				t.Fatalf("got %d validation errors, want %d: %+v",
					len(validErrs), len(tc.expected.validErrFields), validErrs)
			}
			for i, field := range tc.expected.validErrFields {
				if validErrs[i].Field != field {
					t.Errorf("validation error %d: field = %q, want %q",
						i, validErrs[i].Field, field)
				}
			}
		})
	}
}

// TestResolveTemplateMetaUpdate_NameClearedFallsBackToTemplateName covers the
// pre-existing rule that a template's name cannot be cleared from the UI.
// Even an explicit empty pointer must resolve to the existing template name.
func TestResolveTemplateMetaUpdate_NameClearedFallsBackToTemplateName(t *testing.T) {
	t.Parallel()

	tpl := baselineTemplate()
	schedOpts := baselineScheduleOpts()

	got, _ := resolveTemplateMetaUpdate(tpl, schedOpts, codersdk.UpdateTemplateMeta{
		Name: ptr.Ref(""),
	})
	if got.name != tpl.Name {
		t.Fatalf("got name = %q, want %q (preserved)", got.name, tpl.Name)
	}
}

// TestResolveTemplateMetaUpdate_NilRequestUsesScheduleOptsForRequirements
// verifies that an entirely empty request returns the schedule store's
// current autostop/autostart requirement values, rather than zeros.
func TestResolveTemplateMetaUpdate_NilRequestUsesScheduleOptsForRequirements(t *testing.T) {
	t.Parallel()

	tpl := baselineTemplate()
	schedOpts := schedule.TemplateScheduleOptions{
		AutostopRequirement: schedule.TemplateAutostopRequirement{
			DaysOfWeek: 0b0001100, // Wed + Thu
			Weeks:      3,
		},
		AutostartRequirement: schedule.TemplateAutostartRequirement{
			DaysOfWeek: 0b0010000, // Fri
		},
	}

	got, validErrs := resolveTemplateMetaUpdate(tpl, schedOpts, codersdk.UpdateTemplateMeta{})
	if len(validErrs) != 0 {
		t.Fatalf("unexpected validation errors: %+v", validErrs)
	}
	if got.autostopRequirementDaysOfWeekParsed != schedOpts.AutostopRequirement.DaysOfWeek {
		t.Errorf("autostop days = 0b%07b, want 0b%07b",
			got.autostopRequirementDaysOfWeekParsed,
			schedOpts.AutostopRequirement.DaysOfWeek)
	}
	if got.autostartRequirementDaysOfWeekParsed != schedOpts.AutostartRequirement.DaysOfWeek {
		t.Errorf("autostart days = 0b%07b, want 0b%07b",
			got.autostartRequirementDaysOfWeekParsed,
			schedOpts.AutostartRequirement.DaysOfWeek)
	}
	if got.autostopRequirementWeeks != schedOpts.AutostopRequirement.Weeks {
		t.Errorf("autostop weeks = %d, want %d",
			got.autostopRequirementWeeks, schedOpts.AutostopRequirement.Weeks)
	}
}
