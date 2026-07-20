package license

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestBetterUserLimit(t *testing.T) {
	t.Parallel()

	cand := func(limit int64, entitlement codersdk.Entitlement, addon bool) userLimitCandidate {
		return userLimitCandidate{limit: limit, entitlement: entitlement, aiGovernanceAddon: addon}
	}
	entitled := codersdk.EntitlementEntitled
	grace := codersdk.EntitlementGracePeriod

	cases := []struct {
		name           string
		a, b           userLimitCandidate
		countA, countB int64
		want           bool
	}{
		{
			name: "ComplianceBeatsEntitlement",
			a:    cand(200, grace, false), countA: 150,
			b: cand(100, entitled, false), countB: 150,
			want: true,
		},
		{
			name: "ComplianceBeatsHigherLimit",
			a:    cand(100, entitled, true), countA: 90,
			b: cand(200, entitled, false), countB: 250,
			want: true,
		},
		{
			name: "EntitlementBeatsLimitWhenBothCompliant",
			a:    cand(100, entitled, false), countA: 50,
			b: cand(200, grace, false), countB: 50,
			want: true,
		},
		{
			name: "HigherLimitWinsWhenBothCompliantAndEqualEntitlement",
			a:    cand(200, entitled, false), countA: 50,
			b: cand(100, entitled, false), countB: 50,
			want: true,
		},
		{
			name: "HigherLimitWinsWhenBothOver",
			a:    cand(200, entitled, false), countA: 250,
			b: cand(100, entitled, true), countB: 150,
			want: true,
		},
		{
			name: "AddonBreaksExactTies",
			a:    cand(100, entitled, true), countA: 50,
			b: cand(100, entitled, false), countB: 80,
			want: true,
		},
		{
			name: "EqualCandidatesAreNotBetter",
			a:    cand(100, entitled, false), countA: 50,
			b: cand(100, entitled, false), countB: 50,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, betterUserLimit(tc.a, tc.b, tc.countA, tc.countB))
			if tc.want {
				require.False(t, betterUserLimit(tc.b, tc.a, tc.countB, tc.countA),
					"strict ordering must not hold both ways")
			}
		})
	}
}
