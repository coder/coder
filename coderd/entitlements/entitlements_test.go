package entitlements_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestModify(t *testing.T) {
	t.Parallel()

	set := entitlements.New()
	require.False(t, set.Enabled(codersdk.FeatureMultipleOrganizations))

	set.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.HasLicense = true
		entitlements.Features[codersdk.FeatureMultipleOrganizations] = codersdk.Feature{
			Enabled:     true,
			Entitlement: codersdk.EntitlementEntitled,
			Limit:       new(int64(10)),
		}
	})
	require.True(t, set.Enabled(codersdk.FeatureMultipleOrganizations))

	var full codersdk.Entitlements
	require.NoError(t, json.Unmarshal(set.AsJSON(), &full))
	require.NotNil(t, full.Features[codersdk.FeatureMultipleOrganizations].Limit)
	var public codersdk.DeploymentCapabilities
	require.NoError(t, json.Unmarshal(set.AsCapabilitiesJSON(), &public))
	require.True(t, public.Features[codersdk.FeatureMultipleOrganizations].Usable)
	require.NotContains(t, string(set.AsCapabilitiesJSON()), "\"limit\"")
}

func TestAllowRefresh(t *testing.T) {
	t.Parallel()

	now := time.Now()
	set := entitlements.New()
	set.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.RefreshedAt = now
	})

	ok, wait := set.AllowRefresh(now)
	require.False(t, ok)
	require.InDelta(t, time.Minute.Seconds(), wait.Seconds(), 5)

	set.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.RefreshedAt = now.Add(time.Minute * -2)
	})

	ok, wait = set.AllowRefresh(now)
	require.True(t, ok)
	require.Equal(t, time.Duration(0), wait)
}

func TestUpdate(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	set := entitlements.New()
	require.False(t, set.Enabled(codersdk.FeatureMultipleOrganizations))
	fetchStarted := make(chan struct{})
	firstDone := make(chan struct{})
	errCh := make(chan error, 2)
	go func() {
		err := set.Update(ctx, func(_ context.Context) (codersdk.Entitlements, error) {
			close(fetchStarted)
			select {
			case <-firstDone:
				// OK!
			case <-ctx.Done():
				t.Error("timeout")
				return codersdk.Entitlements{}, ctx.Err()
			}
			return codersdk.Entitlements{
				Features: map[codersdk.FeatureName]codersdk.Feature{
					codersdk.FeatureMultipleOrganizations: {
						Enabled: true,
					},
				},
			}, nil
		})
		errCh <- err
	}()
	testutil.TryReceive(ctx, t, fetchStarted)
	require.False(t, set.Enabled(codersdk.FeatureMultipleOrganizations))
	// start a second update while the first one is in progress
	go func() {
		err := set.Update(ctx, func(_ context.Context) (codersdk.Entitlements, error) {
			return codersdk.Entitlements{
				Features: map[codersdk.FeatureName]codersdk.Feature{
					codersdk.FeatureMultipleOrganizations: {
						Enabled: true,
					},
					codersdk.FeatureAppearance: {
						Entitlement: codersdk.EntitlementEntitled,
						Enabled:     true,
					},
				},
				HasLicense: true,
			}, nil
		})
		errCh <- err
	}()
	close(firstDone)
	err := testutil.TryReceive(ctx, t, errCh)
	require.NoError(t, err)
	err = testutil.TryReceive(ctx, t, errCh)
	require.NoError(t, err)
	require.True(t, set.Enabled(codersdk.FeatureMultipleOrganizations))
	require.True(t, set.Enabled(codersdk.FeatureAppearance))
	var public codersdk.DeploymentCapabilities
	require.NoError(t, json.Unmarshal(set.AsCapabilitiesJSON(), &public))
	require.True(t, public.Features[codersdk.FeatureAppearance].Usable)
}

func TestUpdate_LicenseRequiresTelemetry(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	set := entitlements.New()
	set.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.Errors = []string{"some error"}
		entitlements.HasLicense = true
		entitlements.Features[codersdk.FeatureAppearance] = codersdk.Feature{
			Entitlement: codersdk.EntitlementEntitled,
			Enabled:     true,
		}
	})
	err := set.Update(ctx, func(_ context.Context) (codersdk.Entitlements, error) {
		return codersdk.Entitlements{}, entitlements.ErrLicenseRequiresTelemetry
	})
	require.NoError(t, err)
	require.True(t, set.Enabled(codersdk.FeatureAppearance))
	require.Equal(t, []string{entitlements.ErrLicenseRequiresTelemetry.Error()}, set.Errors())
	var public codersdk.DeploymentCapabilities
	require.NoError(t, json.Unmarshal(set.AsCapabilitiesJSON(), &public))
	require.True(t, public.Features[codersdk.FeatureAppearance].Usable)
	require.NotContains(t, string(set.AsCapabilitiesJSON()), entitlements.ErrLicenseRequiresTelemetry.Error())
}
