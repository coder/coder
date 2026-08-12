package confine

import (
	"context"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubnetAllocator(t *testing.T) {
	t.Parallel()

	pool := netip.MustParsePrefix("192.0.2.0/29")
	tests := []struct {
		name        string
		used        []netip.Prefix
		allocations int
		want        []subnetLease
		wantErr     string
	}{
		{
			name:        "allocates sequential subnets",
			allocations: 2,
			want: []subnetLease{
				{
					Prefix: netip.MustParsePrefix("192.0.2.0/30"),
					Host:   netip.MustParseAddr("192.0.2.1"),
					Peer:   netip.MustParseAddr("192.0.2.2"),
				},
				{
					Prefix: netip.MustParsePrefix("192.0.2.4/30"),
					Host:   netip.MustParseAddr("192.0.2.5"),
					Peer:   netip.MustParseAddr("192.0.2.6"),
				},
			},
		},
		{
			name: "skips route collision",
			used: []netip.Prefix{
				netip.MustParsePrefix("192.0.2.0/31"),
			},
			allocations: 1,
			want: []subnetLease{
				{
					Prefix: netip.MustParsePrefix("192.0.2.4/30"),
					Host:   netip.MustParseAddr("192.0.2.5"),
					Peer:   netip.MustParseAddr("192.0.2.6"),
				},
			},
		},
		{
			name: "skips interface address collision",
			used: []netip.Prefix{
				netip.MustParsePrefix("192.0.2.2/32"),
			},
			allocations: 1,
			want: []subnetLease{
				{
					Prefix: netip.MustParsePrefix("192.0.2.4/30"),
					Host:   netip.MustParseAddr("192.0.2.5"),
					Peer:   netip.MustParseAddr("192.0.2.6"),
				},
			},
		},
		{
			name: "exhausted by collisions",
			used: []netip.Prefix{
				netip.MustParsePrefix("192.0.2.0/29"),
			},
			allocations: 1,
			wantErr:     `netns subnet pool "192.0.2.0/29" is exhausted`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			allocator := newSubnetAllocator(func(context.Context) ([]netip.Prefix, error) {
				return tt.used, nil
			})
			var got []subnetLease
			for range tt.allocations {
				lease, err := allocator.Allocate(t.Context(), pool)
				if tt.wantErr != "" {
					require.EqualError(t, err, tt.wantErr)
					return
				}
				require.NoError(t, err)
				got = append(got, lease)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSubnetAllocatorExhaustion(t *testing.T) {
	t.Parallel()

	allocator := newSubnetAllocator(func(context.Context) ([]netip.Prefix, error) {
		return nil, nil
	})
	pool := netip.MustParsePrefix("192.0.2.0/30")

	_, err := allocator.Allocate(t.Context(), pool)
	require.NoError(t, err)
	_, err = allocator.Allocate(t.Context(), pool)
	require.EqualError(t, err, `netns subnet pool "192.0.2.0/30" is exhausted`)
}

func TestSubnetAllocatorReleaseReuse(t *testing.T) {
	t.Parallel()

	allocator := newSubnetAllocator(func(context.Context) ([]netip.Prefix, error) {
		return nil, nil
	})
	pool := netip.MustParsePrefix("192.0.2.0/29")

	first, err := allocator.Allocate(t.Context(), pool)
	require.NoError(t, err)
	second, err := allocator.Allocate(t.Context(), pool)
	require.NoError(t, err)
	require.NotEqual(t, first.Prefix, second.Prefix)

	allocator.Release(first.Prefix)
	reused, err := allocator.Allocate(t.Context(), pool)
	require.NoError(t, err)
	require.Equal(t, first, reused)
}

func TestSubnetAllocatorListerError(t *testing.T) {
	t.Parallel()

	allocator := newSubnetAllocator(func(context.Context) ([]netip.Prefix, error) {
		return nil, context.Canceled
	})
	_, err := allocator.Allocate(t.Context(), netip.MustParsePrefix("192.0.2.0/30"))
	require.ErrorContains(t, err, "list host network prefixes: context canceled")
}

func TestSubnetAllocatorRejectsInvalidPools(t *testing.T) {
	t.Parallel()

	allocator := newSubnetAllocator(func(context.Context) ([]netip.Prefix, error) {
		return nil, nil
	})

	_, err := allocator.Allocate(t.Context(), netip.MustParsePrefix("2001:db8::/64"))
	require.EqualError(t, err, `netns subnet pool "2001:db8::/64" is not IPv4`)

	_, err = allocator.Allocate(t.Context(), netip.MustParsePrefix("192.0.2.0/31"))
	require.EqualError(t, err, `netns subnet pool "192.0.2.0/31" is smaller than /30`)
}
