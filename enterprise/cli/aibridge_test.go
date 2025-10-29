package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestAIBridgeListInterceptions(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAIBridge: 1,
				},
			},
		})
		memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		now := dbtime.Now()
		interception1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: member.ID,
			StartedAt:   now.Add(-time.Hour),
		}, &now)
		interception2 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: member.ID,
			StartedAt:   now,
		}, nil)
		// Should not be returned because the user can't see it.
		_ = dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: owner.UserID,
			StartedAt:   now.Add(-2 * time.Hour),
		}, nil)

		args := []string{
			"aibridge",
			"interceptions",
			"list",
		}
		inv, root := newCLI(t, args...)
		clitest.SetupConfig(t, memberClient, root)

		ctx := testutil.Context(t, testutil.WaitLong)

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Reverse order because the order is `started_at ASC`.
		requireHasInterceptions(t, out.Bytes(), []uuid.UUID{interception2.ID, interception1.ID})
	})

	t.Run("Filter", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAIBridge: 1,
				},
			},
		})
		memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		now := dbtime.Now()

		// This interception should be returned since it matches all filters.
		goodInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: member.ID,
			Provider:    "real-provider",
			Model:       "real-model",
			StartedAt:   now,
		}, nil)

		// These interceptions should not be returned since they don't match the
		// filters.
		_ = dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: owner.UserID,
			Provider:    goodInterception.Provider,
			Model:       goodInterception.Model,
			StartedAt:   goodInterception.StartedAt,
		}, nil)
		_ = dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: goodInterception.InitiatorID,
			Provider:    "bad-provider",
			Model:       goodInterception.Model,
			StartedAt:   goodInterception.StartedAt,
		}, nil)
		_ = dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: goodInterception.InitiatorID,
			Provider:    goodInterception.Provider,
			Model:       "bad-model",
			StartedAt:   goodInterception.StartedAt,
		}, nil)
		_ = dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: goodInterception.InitiatorID,
			Provider:    goodInterception.Provider,
			Model:       goodInterception.Model,
			// Violates the started after filter.
			StartedAt: now.Add(-2 * time.Hour),
		}, nil)
		_ = dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: goodInterception.InitiatorID,
			Provider:    goodInterception.Provider,
			Model:       goodInterception.Model,
			// Violates the started before filter.
			StartedAt: now.Add(2 * time.Hour),
		}, nil)

		args := []string{
			"aibridge",
			"interceptions",
			"list",
			"--started-after", now.Add(-time.Hour).Format(time.RFC3339),
			"--started-before", now.Add(time.Hour).Format(time.RFC3339),
			"--initiator", codersdk.Me,
			"--provider", goodInterception.Provider,
			"--model", goodInterception.Model,
		}
		inv, root := newCLI(t, args...)
		clitest.SetupConfig(t, memberClient, root)

		ctx := testutil.Context(t, testutil.WaitLong)

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		requireHasInterceptions(t, out.Bytes(), []uuid.UUID{goodInterception.ID})
	})

	t.Run("Pagination", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAIBridge: 1,
				},
			},
		})
		memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		now := dbtime.Now()
		firstInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: member.ID,
			StartedAt:   now,
		}, nil)
		returnedInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: member.ID,
			StartedAt:   now.Add(-time.Hour),
		}, &now)
		_ = dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: member.ID,
			StartedAt:   now.Add(-2 * time.Hour),
		}, nil)

		args := []string{
			"aibridge",
			"interceptions",
			"list",
			"--limit", "1",
			"--after-id", firstInterception.ID.String(),
		}
		inv, root := newCLI(t, args...)
		clitest.SetupConfig(t, memberClient, root)

		ctx := testutil.Context(t, testutil.WaitLong)

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Only contains the second interception because after_id is the first
		// interception, and we set a limit of 1.
		requireHasInterceptions(t, out.Bytes(), []uuid.UUID{returnedInterception.ID})
	})
}

func requireHasInterceptions(t *testing.T, out []byte, ids []uuid.UUID) {
	t.Helper()

	var results []codersdk.AIBridgeInterception
	require.NoError(t, json.Unmarshal(out, &results))
	require.Len(t, results, len(ids))
	for i, id := range ids {
		require.Equal(t, id, results[i].ID)
	}
}
