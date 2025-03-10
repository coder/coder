package prebuilds_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tailscale.com/types/ptr"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/testutil"
)

func TestMetricsCollector(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test requires postgres")
	}

	type testCase struct {
		name                             string
		transitions                      []database.WorkspaceTransition
		jobStatuses                      []database.ProvisionerJobStatus
		initiatorIDs                     []uuid.UUID
		ownerIDs                         []uuid.UUID
		shouldIncrementPrebuildsCreated  *bool
		shouldIncrementPrebuildsFailed   *bool
		shouldIncrementPrebuildsAssigned *bool
	}

	tests := []testCase{
		{
			name: "prebuild created",
			// A prebuild is a workspace, for which the first build was a start transition
			// initiated by the prebuilds user. Whether or not the build was successful, it
			// is still a prebuild. It might just not be a running prebuild.
			transitions:                     allTransitions,
			jobStatuses:                     allJobStatuses,
			initiatorIDs:                    []uuid.UUID{prebuilds.OwnerID},
			ownerIDs:                        []uuid.UUID{prebuilds.OwnerID, uuid.New()},
			shouldIncrementPrebuildsCreated: ptr.To(true),
		},
		{
			name:                            "prebuild failed",
			transitions:                     allTransitions,
			jobStatuses:                     []database.ProvisionerJobStatus{database.ProvisionerJobStatusFailed},
			initiatorIDs:                    []uuid.UUID{prebuilds.OwnerID},
			ownerIDs:                        []uuid.UUID{prebuilds.OwnerID, uuid.New()},
			shouldIncrementPrebuildsCreated: ptr.To(true),
			shouldIncrementPrebuildsFailed:  ptr.To(true),
		},
		{
			name:                             "prebuild assigned",
			transitions:                      allTransitions,
			jobStatuses:                      allJobStatuses,
			initiatorIDs:                     []uuid.UUID{prebuilds.OwnerID},
			ownerIDs:                         []uuid.UUID{uuid.New()},
			shouldIncrementPrebuildsCreated:  ptr.To(true),
			shouldIncrementPrebuildsAssigned: ptr.To(true),
		},
		{
			name:                             "workspaces that were not created by the prebuilds user are not counted",
			transitions:                      allTransitions,
			jobStatuses:                      allJobStatuses,
			initiatorIDs:                     []uuid.UUID{uuid.New()},
			ownerIDs:                         []uuid.UUID{uuid.New()},
			shouldIncrementPrebuildsCreated:  ptr.To(false),
			shouldIncrementPrebuildsFailed:   ptr.To(false),
			shouldIncrementPrebuildsAssigned: ptr.To(false),
		},
	}
	for _, test := range tests {
		for _, transition := range test.transitions {
			for _, jobStatus := range test.jobStatuses {
				for _, initiatorID := range test.initiatorIDs {
					for _, ownerID := range test.ownerIDs {
						t.Run(fmt.Sprintf("transition:%s/jobStatus:%s", transition, jobStatus), func(t *testing.T) {
							t.Parallel()

							logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
							t.Cleanup(func() {
								if t.Failed() {
									t.Logf("failed to run test: %s", test.name)
									t.Logf("transition: %s", transition)
									t.Logf("jobStatus: %s", jobStatus)
									t.Logf("initiatorID: %s", initiatorID)
									t.Logf("ownerID: %s", ownerID)
								}
							})
							db, pubsub := dbtestutil.NewDB(t)
							ctx := testutil.Context(t, testutil.WaitLong)

							createdUsers := []uuid.UUID{prebuilds.OwnerID}
							for _, user := range slices.Concat(test.ownerIDs, test.initiatorIDs) {
								if !slices.Contains(createdUsers, user) {
									dbgen.User(t, db, database.User{
										ID: user,
									})
									createdUsers = append(createdUsers, user)
								}
							}

							collector := prebuilds.NewMetricsCollector(db, logger)

							iterations := 3
							for i := 0; i < iterations; i++ {
								orgID, templateID := setupTestDBTemplate(t, db, createdUsers[0])
								templateVersionID := setupTestDBTemplateVersion(t, ctx, db, pubsub, orgID, createdUsers[0], templateID)
								presetID := setupTestDBPreset(t, ctx, db, pubsub, templateVersionID)
								setupTestDBPrebuild(
									t, ctx, db, pubsub,
									transition, jobStatus, orgID, templateID, templateVersionID, presetID, initiatorID, ownerID,
								)
							}

							if test.shouldIncrementPrebuildsCreated != nil {
								createdCount := promtestutil.CollectAndCount(collector, "coderd_prebuilds_created")
								require.Equal(t, *test.shouldIncrementPrebuildsCreated, createdCount == iterations, "createdCount: %d", createdCount)
							}

							if test.shouldIncrementPrebuildsFailed != nil {
								failedCount := promtestutil.CollectAndCount(collector, "coderd_prebuilds_failed")
								require.Equal(t, *test.shouldIncrementPrebuildsFailed, failedCount == iterations, "failedCount: %d", failedCount)
							}

							if test.shouldIncrementPrebuildsAssigned != nil {
								assignedCount := promtestutil.CollectAndCount(collector, "coderd_prebuilds_assigned")
								require.Equal(t, *test.shouldIncrementPrebuildsAssigned, assignedCount == iterations, "assignedCount: %d", assignedCount)
							}
						})
					}
				}
			}
		}
	}
}
