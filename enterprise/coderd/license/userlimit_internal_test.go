package license

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestBetterUserLimit(t *testing.T) {
	t.Parallel()

	cand := func(limit, count int64, entitlement codersdk.Entitlement, addon bool) resolvedCandidate {
		return resolvedCandidate{
			userLimitCandidate: userLimitCandidate{limit: limit, entitlement: entitlement, aiGovernanceAddon: addon},
			count:              count,
		}
	}
	entitled := codersdk.EntitlementEntitled
	grace := codersdk.EntitlementGracePeriod

	cases := []struct {
		name string
		a, b resolvedCandidate
		want bool
	}{
		{
			name: "ComplianceBeatsEntitlement",
			a:    cand(200, 150, grace, false),
			b:    cand(100, 150, entitled, false),
			want: true,
		},
		{
			name: "ComplianceBeatsHigherLimit",
			a:    cand(100, 90, entitled, true),
			b:    cand(200, 250, entitled, false),
			want: true,
		},
		{
			name: "EntitlementBeatsLimitWhenBothCompliant",
			a:    cand(100, 50, entitled, false),
			b:    cand(200, 50, grace, false),
			want: true,
		},
		{
			name: "HigherLimitWinsWhenBothCompliantAndEqualEntitlement",
			a:    cand(200, 50, entitled, false),
			b:    cand(100, 50, entitled, false),
			want: true,
		},
		{
			name: "HigherLimitWinsWhenBothOver",
			a:    cand(200, 250, entitled, false),
			b:    cand(100, 150, entitled, true),
			want: true,
		},
		{
			name: "AddonBreaksExactTies",
			a:    cand(100, 50, entitled, true),
			b:    cand(100, 80, entitled, false),
			want: true,
		},
		{
			name: "EqualCandidatesAreNotBetter",
			a:    cand(100, 50, entitled, false),
			b:    cand(100, 50, entitled, false),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, betterUserLimit(tc.a, tc.b))
			if tc.want {
				require.False(t, betterUserLimit(tc.b, tc.a),
					"strict ordering must not hold both ways")
			}
		})
	}
}
