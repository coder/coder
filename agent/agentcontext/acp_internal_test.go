package agentcontext

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestACPContextResources(t *testing.T) {
	t.Parallel()
	resolver := &Resolver{ACPResources: func() []Resource { return []Resource{testACPHarnessResource()} }}
	snap := resolver.Resolve(nil)
	require.Len(t, snap.Resources, 1)
	req := snapshotToPushRequest(snap, true)
	require.Equal(t, snap.Resources, req.Resources)
	require.Equal(t, ComputeAggregateHash(snap.Resources), req.AggregateHash)
	require.Empty(t, req.SnapshotError)
	encoded := pushRequestToProto(req)
	require.Len(t, encoded.Resources, 1)
	require.Equal(t, "/home/coder/.coder/acp", encoded.Resources[0].GetSourcePath())
	body := encoded.Resources[0].GetAcpHarness()
	require.NotNil(t, body)
	require.Equal(t, "fake", body.Slug)
	require.True(t, body.Steering)
	require.Equal(t, "default", body.ConfigOptions[0].CurrentValue)
	require.Equal(t, "Default model", body.ConfigOptions[0].Values[0].Description)
}

func TestACPContextLimits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name             string
		maxResources     int
		maxSnapshotBytes uint64
		maxResourceBytes uint64
		mcp              bool
		wantStatus       ResourceStatus
	}{
		{name: "Included", maxResources: 2, maxSnapshotBytes: 32, maxResourceBytes: 16, wantStatus: StatusOK},
		{name: "Count", maxResources: 1, maxSnapshotBytes: 32, maxResourceBytes: 16, wantStatus: StatusExcluded},
		{name: "AggregateBytes", maxResources: 2, maxSnapshotBytes: 16, maxResourceBytes: 16, wantStatus: StatusExcluded},
		{name: "ResourceBytes", maxResources: 2, maxSnapshotBytes: 32, maxResourceBytes: 12, wantStatus: StatusOversize},
		{name: "MCPAggregateBytes", maxResources: 3, maxSnapshotBytes: 32, maxResourceBytes: 16, mcp: true, wantStatus: StatusExcluded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("instructions"), 0o600))
			resolver := &Resolver{
				MaxResources:     tc.maxResources,
				MaxSnapshotBytes: tc.maxSnapshotBytes,
				MaxResourceBytes: tc.maxResourceBytes,
				ACPResources:     func() []Resource { return []Resource{testACPHarnessResource()} },
			}
			if tc.mcp {
				resolver.MCPResources = func() []Resource {
					return []Resource{{ID: "mcp_server:fake", Kind: KindMCPServer, Source: "fake", Status: StatusOK, Payload: []byte("tool list")}}
				}
			}
			snap := resolver.Resolve([]ScanRoot{{Path: dir}})
			harness := snap.Resources[0]
			require.Equal(t, KindACPHarness, harness.Kind)
			require.Equal(t, tc.wantStatus, harness.Status)
			if tc.wantStatus == StatusOK {
				require.NotNil(t, harness.ACPHarness)
			} else {
				require.Nil(t, harness.ACPHarness)
				require.Empty(t, harness.Payload)
			}
			require.Equal(t, KindInstructionFile, snap.Resources[1].Kind)
			require.Equal(t, StatusOK, snap.Resources[1].Status)
			if tc.mcp {
				require.Equal(t, StatusOK, snap.Resources[2].Status)
			}
			req := snapshotToPushRequest(snap, true)
			require.Equal(t, snap.Resources, req.Resources)
			require.Equal(t, ComputeAggregateHash(driftResources(snap.Resources)), req.AggregateHash)
			require.NotNil(t, pushRequestToProto(req).Resources[0].GetAcpHarness())
		})
	}
}

func testACPHarnessResource() Resource {
	return Resource{
		ID:          "acp_harness:fake",
		Kind:        KindACPHarness,
		Source:      "fake",
		SourcePath:  "/home/coder/.coder/acp",
		Status:      StatusOK,
		Payload:     []byte("cached metadata"),
		SizeBytes:   15,
		ContentHash: sha256.Sum256([]byte("cached metadata")),
		ACPHarness: &workspacesdk.ACPHarness{
			Slug: "fake", DisplayName: "Fake", LoadSession: true, ResumeSession: true, Steering: true,
			ConfigOptions: []workspacesdk.ACPConfigOption{{
				ID: "model", Name: "Model", Description: "A model", CurrentValue: "default",
				Values: []workspacesdk.ACPConfigValue{{ID: "default", Name: "Default", Description: "Default model", Group: "group"}},
			}},
		},
	}
}
