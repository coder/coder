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
}
