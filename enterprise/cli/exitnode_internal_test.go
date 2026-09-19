package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestExitNodeListRows(t *testing.T) {
	t.Parallel()

	nodes := []codersdk.ExitNode{{
		Name:   "egress",
		Status: codersdk.ExitNodeStatusHealthy,
		Replicas: []codersdk.ExitNodeReplica{
			{Status: codersdk.ExitNodeReplicaStatusLive},
			{Status: codersdk.ExitNodeReplicaStatusLive},
			{Status: codersdk.ExitNodeReplicaStatusStale},
			{Status: codersdk.ExitNodeReplicaStatusStopped},
		},
	}}
	rows := exitNodeListRows(nodes)
	require.Len(t, rows, 1)
	require.Equal(t, nodes[0], rows[0].ExitNode)
	require.Equal(t, 2, rows[0].LiveReplicas)
}

func TestExitNodeReplicaRows(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, time.September, 19, 12, 34, 56, 0, time.UTC)
	replicas := []codersdk.ExitNodeReplica{{
		Hostname:       "exit-1",
		Status:         codersdk.ExitNodeReplicaStatusLive,
		Version:        "v1.2.3",
		TailnetAddress: "fd7a:115c:a1e0::1",
		PolicyHash:     "1234567890abcdef",
		UpdatedAt:      updatedAt,
	}}
	rows := exitNodeReplicaRows(replicas)
	require.Equal(t, []exitNodeReplicaRow{{
		Hostname:       "exit-1",
		Status:         codersdk.ExitNodeReplicaStatusLive,
		Version:        "v1.2.3",
		TailnetAddress: "fd7a:115c:a1e0::1",
		PolicyHash:     "1234567890ab",
		UpdatedAt:      "2026-09-19 12:34:56",
	}}, rows)
}
