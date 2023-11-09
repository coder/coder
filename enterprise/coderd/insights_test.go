package coderd_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateInsightsWithTemplateAdminACL(t *testing.T) {
	t.Parallel()

	y, m, d := time.Now().UTC().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	type test struct {
		interval codersdk.InsightsReportInterval
	}

	tests := []test{
		{codersdk.InsightsReportIntervalDay},
		{""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(fmt.Sprintf("with interval=%q", tt.interval), func(t *testing.T) {
			t.Parallel()

			client, admin := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			}})
			templateAdminClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleTemplateAdmin())

			version := coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID)

			regular, regularUser := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			err := templateAdminClient.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
				UserPerms: map[string]codersdk.TemplateRole{
					regularUser.ID.String(): codersdk.TemplateRoleAdmin,
				},
			})
			require.NoError(t, err)

			_, err = regular.TemplateInsights(ctx, codersdk.TemplateInsightsRequest{
				StartTime:   today.AddDate(0, 0, -1),
				EndTime:     today,
				TemplateIDs: []uuid.UUID{template.ID},
			})
			require.NoError(t, err)
		})
	}
}
