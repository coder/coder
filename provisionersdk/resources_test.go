package provisionersdk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestResourceAddresses(t *testing.T) {
	t.Parallel()
	t.Run("Single", func(t *testing.T) {
		addresses, err := provisionersdk.ResourceAddresses([]*proto.Resource{{
			Type: "google_compute_instance",
			Name: "dev",
		}})
		require.NoError(t, err)
		require.Len(t, addresses, 1)
		require.Equal(t, addresses[0], "dev")
	})
	t.Run("Multiple", func(t *testing.T) {
		addresses, err := provisionersdk.ResourceAddresses([]*proto.Resource{{
			Type: "google_compute_instance",
			Name: "linux",
		}, {
			Type: "google_compute_instance",
			Name: "windows",
		}})
		require.NoError(t, err)
		require.Len(t, addresses, 2)
		require.Equal(t, addresses[0], "linux")
		require.Equal(t, addresses[1], "windows")
	})
	t.Run("ConflictingDifferent", func(t *testing.T) {
		addresses, err := provisionersdk.ResourceAddresses([]*proto.Resource{{
			Type: "google_compute_instance",
			Name: "dev",
		}, {
			Type: "kubernetes_pod",
			Name: "dev",
		}})
		require.NoError(t, err)
		require.Len(t, addresses, 2)
		require.Equal(t, addresses[0], "google_compute_instance.dev")
		require.Equal(t, addresses[1], "kubernetes_pod.dev")
	})
	t.Run("ConflictingSame", func(t *testing.T) {
		_, err := provisionersdk.ResourceAddresses([]*proto.Resource{{
			Type: "google_compute_instance",
			Name: "dev",
		}, {
			Type: "google_compute_instance",
			Name: "dev",
		}})
		require.Error(t, err)
	})
}
