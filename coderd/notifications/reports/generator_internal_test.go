package reports

import "testing"

func TestReportFailedWorkspaceBuilds(t *testing.T) {
	t.Parallel()

	t.Run("FailedBuilds_TemplateAdminOptIn_FirstRun_Report", func(t *testing.T) {
		t.Parallel()
		// TODO
	})

	t.Run("FailedBuilds_TemplateAdminOptIn_SecondRunTooEarly_NoReport", func(t *testing.T) {
		t.Parallel()
		// TODO
	})

	t.Run("FailedBuilds_TemplateAdminOptIn_SecondRun_Report", func(t *testing.T) {
		t.Parallel()
		// TODO
	})

	t.Run("NoFailedBuilds_TemplateAdminIn_NoReport", func(t *testing.T) {
		t.Parallel()
		// TODO
	})

	t.Run("FailedBuilds_TemplateAdminOptOut_NoReport", func(t *testing.T) {
		t.Parallel()
		// TODO
	})

	t.Run("StaleFailedBuilds_TemplateAdminOptIn_NoReport_Cleanup", func(t *testing.T) {
		t.Parallel()
		// TODO
	})
}
