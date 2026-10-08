package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/codersdk"
)

func Test_templateEditWorkspaceImpactingChanges(t *testing.T) {
	t.Parallel()

	newReq := func(mutate func(*codersdk.UpdateTemplateMeta)) codersdk.UpdateTemplateMeta {
		req := codersdk.UpdateTemplateMeta{
			FailureTTLMillis:               new(int64),
			TimeTilDormantMillis:           new(int64),
			TimeTilDormantAutoDeleteMillis: new(int64),
			RequireActiveVersion:           new(bool),
			DefaultTTLMillis:               new(int64),
			AllowUserAutostop:              new(true),
			AutostartRequirement: &codersdk.TemplateAutostartRequirement{
				DaysOfWeek: []string{"monday", "tuesday"},
			},
			AutostopRequirement: &codersdk.TemplateAutostopRequirement{
				DaysOfWeek: []string{},
				Weeks:      1,
			},
		}
		if mutate != nil {
			mutate(&req)
		}
		return req
	}

	template := codersdk.Template{
		AllowUserAutostop: true,
		AutostartRequirement: codersdk.TemplateAutostartRequirement{
			DaysOfWeek: []string{"tuesday", "monday"},
		},
		AutostopRequirement: codersdk.TemplateAutostopRequirement{
			DaysOfWeek: []string{},
			Weeks:      1,
		},
	}

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		changes := templateEditWorkspaceImpactingChanges(template, newReq(nil))
		assert.Empty(t, changes)
	})

	t.Run("ZeroWeeksMatchesNormalizedTemplate", func(t *testing.T) {
		t.Parallel()

		// A requested value of 0 (e.g. from an unset flag or an explicit
		// "--autostop-requirement-weeks 0") is normalized by the server to 1,
		// the same value already on the template, so it's a no-op and should
		// not be reported as a change.
		req := newReq(func(r *codersdk.UpdateTemplateMeta) {
			r.AutostopRequirement.Weeks = 0
		})

		changes := templateEditWorkspaceImpactingChanges(template, req)
		assert.Empty(t, changes)
	})

	t.Run("NegativeWeeksMatchesNormalizedTemplate", func(t *testing.T) {
		t.Parallel()

		req := newReq(func(r *codersdk.UpdateTemplateMeta) {
			r.AutostopRequirement.Weeks = -1
		})

		changes := templateEditWorkspaceImpactingChanges(template, req)
		assert.Empty(t, changes)
	})

	t.Run("WeeksActuallyChanged", func(t *testing.T) {
		t.Parallel()

		req := newReq(func(r *codersdk.UpdateTemplateMeta) {
			r.AutostopRequirement.Weeks = 2
		})

		changes := templateEditWorkspaceImpactingChanges(template, req)
		assert.Len(t, changes, 1)
		assert.Contains(t, changes[0], "Autostop requirement weeks")
	})

	t.Run("AutostartDaysChanged", func(t *testing.T) {
		t.Parallel()

		req := newReq(func(r *codersdk.UpdateTemplateMeta) {
			r.AutostartRequirement.DaysOfWeek = []string{"monday"}
		})

		changes := templateEditWorkspaceImpactingChanges(template, req)
		assert.Len(t, changes, 1)
		assert.Contains(t, changes[0], "Autostart requirement days")
	})

	t.Run("WeekdaysDifferOnlyInCaseOrDuplicates", func(t *testing.T) {
		t.Parallel()

		// The server compares weekdays as bitmaps, so case and duplicates
		// don't change the effective value.
		req := newReq(func(r *codersdk.UpdateTemplateMeta) {
			r.AutostartRequirement.DaysOfWeek = []string{"Monday", "TUESDAY", "monday"}
			r.AutostopRequirement.DaysOfWeek = []string{}
		})
		tmpl := template
		tmpl.AutostopRequirement.DaysOfWeek = nil

		changes := templateEditWorkspaceImpactingChanges(tmpl, req)
		assert.Empty(t, changes)
	})

	t.Run("AutostopDaysChangedUsesCanonicalNames", func(t *testing.T) {
		t.Parallel()

		req := newReq(func(r *codersdk.UpdateTemplateMeta) {
			r.AutostopRequirement.DaysOfWeek = []string{"Sunday", "saturday"}
		})

		changes := templateEditWorkspaceImpactingChanges(template, req)
		assert.Equal(t, []string{"Autostop requirement days: [] -> [saturday sunday]"}, changes)
	})

	t.Run("AllowUserAutostopDisabled", func(t *testing.T) {
		t.Parallel()

		req := newReq(func(r *codersdk.UpdateTemplateMeta) {
			r.AllowUserAutostop = new(false)
		})

		changes := templateEditWorkspaceImpactingChanges(template, req)
		assert.Len(t, changes, 1)
		assert.Contains(t, changes[0], "Allow user autostop")
	})

	t.Run("AllowUserAutostopEnabled", func(t *testing.T) {
		t.Parallel()

		disabled := template
		disabled.AllowUserAutostop = false

		changes := templateEditWorkspaceImpactingChanges(disabled, newReq(nil))
		assert.Empty(t, changes)
	})

	t.Run("DefaultTTLChangedWithAutostopAllowed", func(t *testing.T) {
		t.Parallel()

		req := newReq(func(r *codersdk.UpdateTemplateMeta) {
			r.DefaultTTLMillis = new(int64(3600000))
		})

		changes := templateEditWorkspaceImpactingChanges(template, req)
		assert.Empty(t, changes)
	})

	t.Run("DefaultTTLChangedWithAutostopDisabled", func(t *testing.T) {
		t.Parallel()

		disabled := template
		disabled.AllowUserAutostop = false

		req := newReq(func(r *codersdk.UpdateTemplateMeta) {
			r.AllowUserAutostop = new(false)
			r.DefaultTTLMillis = new(int64(3600000))
		})

		changes := templateEditWorkspaceImpactingChanges(disabled, req)
		assert.Len(t, changes, 1)
		assert.Contains(t, changes[0], "Default TTL")
	})

	t.Run("DisablingAutostopAndChangingTTL", func(t *testing.T) {
		t.Parallel()

		req := newReq(func(r *codersdk.UpdateTemplateMeta) {
			r.AllowUserAutostop = new(false)
			r.DefaultTTLMillis = new(int64(3600000))
		})

		changes := templateEditWorkspaceImpactingChanges(template, req)
		assert.Len(t, changes, 2)
	})
}
