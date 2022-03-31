package authz_test

import (
	"fmt"
	"github.com/coder/coder/coderd/authz"
	"github.com/coder/coder/coderd/authz/authztest"
	"github.com/stretchr/testify/require"
	"math/bits"
	"strings"
	"testing"
)

var nilSet = authztest.Set{nil}

func Test_ExhaustiveAuthorize(t *testing.T) {
	all := authztest.GroupedPermissions(authztest.AllPermissions())
	roleVariants := permissionVariants(all)

	testCases := []struct {
		Name string
		Objs []authz.Object
		// Action is constant
		// Subject comes from roleVariants
		Result func(pv string) bool
	}{
		{
			Name: "User:Org",
			Objs: authztest.Objects(
				[]string{authztest.PermMe, authztest.PermOrgID},
			),
			Result: func(pv string) bool {
				return strings.Contains(pv, "+")
			},
		},
		{
			// All U+/- tests should fail
			Name: "NotUser:Org",
			Objs: authztest.Objects(
				[]string{"other", authztest.PermOrgID},
				[]string{"", authztest.PermOrgID},
			),
			Result: func(pv string) bool {
				if strings.Contains(pv, "U") {
					return false
				}
				return strings.Contains(pv, "+")
			},
		},
		{
			// All O+/- and U+/- tests should fail
			Name: "NotUser:NotOrg",
			Objs: authztest.Objects(
				[]string{authztest.PermMe, "non-mem"},
				[]string{"other", "non-mem"},
				[]string{"other", ""},
				[]string{"", "non-mem"},
				[]string{"", ""},
			),
			Result: func(pv string) bool {
				if strings.Contains(pv, "U") {
					return false
				}
				if strings.Contains(pv, "O") {
					return false
				}
				return strings.Contains(pv, "+")
			},
		},
		// TODO: @emyrk for this one, we should probably pass a custom roles variant
		//{
		//	// O+, O- no longer pass judgement. Defer to user level judgement (only somewhat tricky case)
		//	Name: "User:NotOrg",
		//	Objs: authztest.Objects(
		//		[]string{authztest.PermMe, ""},
		//	),
		//	Result: func(pv string) bool {
		//		return strings.Contains(pv, "+")
		//	},
		//},
	}

	var failedTests int
	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			for _, o := range c.Objs {
				for name, v := range roleVariants {
					v.Each(func(set authztest.Set) {
						// TODO: Authz.Permissions does allocations at the moment. We should fix that.
						err := authz.AuthorizePermissions(
							authztest.PermMe,
							set.Permissions(),
							o,
							authztest.PermAction)
						if c.Result(name) && err != nil {
							failedTests++
						} else if !c.Result(name) && err == nil {
							failedTests++
						}
					})
					v.Reset()
				}
			}
		})
	}
	require.Equal(t, 0, failedTests, fmt.Sprintf("%d tests failed", failedTests))
}

func permissionVariants(all authztest.SetGroup) map[string]*authztest.Role {
	// an is any noise above the impactful set
	an := noiseAbstain
	// ln is any noise below the impactful set
	ln := noisePositive | noiseNegative | noiseAbstain

	// Cases are X+/- where X indicates the level where the impactful set is.
	// The impactful set determines the result.
	return map[string]*authztest.Role{
		// Wild
		"W+": authztest.NewRole(
			pos(all.Wildcard()),
			noise(ln, all.Site(), all.Org(), all.User()),
		),
		"W-": authztest.NewRole(
			neg(all.Wildcard()),
			noise(ln, all.Site(), all.Org(), all.User()),
		),
		// Site
		"S+": authztest.NewRole(
			noise(an, all.Wildcard()),
			pos(all.Site()),
			noise(ln, all.Org(), all.User()),
		),
		"S-": authztest.NewRole(
			noise(an, all.Wildcard()),
			neg(all.Site()),
			noise(ln, all.Org(), all.User()),
		),
		// Org:* -- Added org:mem noise
		"O+": authztest.NewRole(
			noise(an, all.Wildcard(), all.Site(), all.OrgMem()),
			pos(all.Org()),
			noise(ln, all.User()),
		),
		"O-": authztest.NewRole(
			noise(an, all.Wildcard(), all.Site(), all.OrgMem()),
			neg(all.Org()),
			noise(ln, all.User()),
		),
		// Org:Mem -- Added org:* noise
		"M+": authztest.NewRole(
			noise(an, all.Wildcard(), all.Site(), all.Org()),
			pos(all.OrgMem()),
			noise(ln, all.User()),
		),
		"M-": authztest.NewRole(
			noise(an, all.Wildcard(), all.Site(), all.Org()),
			neg(all.OrgMem()),
			noise(ln, all.User()),
		),
		// User
		"U+": authztest.NewRole(
			noise(an, all.Wildcard(), all.Site(), all.Org()),
			pos(all.User()),
		),
		"U-": authztest.NewRole(
			noise(an, all.Wildcard(), all.Site(), all.Org()),
			neg(all.User()),
		),
		// TODO: @Emyrk the abstain sets
	}
}

// pos returns the positive impactful variant for a given level. It does not
// include noise at any other level but the one given.
func pos(lvl authztest.LevelGroup) *authztest.Role {
	return authztest.NewRole(
		lvl.Positive(),
		authztest.Union(lvl.Abstain()[:1], nilSet),
	)
}

func neg(lvl authztest.LevelGroup) *authztest.Role {
	return authztest.NewRole(
		lvl.Negative(),
		authztest.Union(lvl.Positive()[:1], nilSet),
		authztest.Union(lvl.Abstain()[:1], nilSet),
	)
}

type noiseBits uint8

const (
	_ noiseBits = 1 << iota
	noisePositive
	noiseNegative
	noiseAbstain
)

func flagMatch(flag, in noiseBits) bool {
	return flag&in != 0
}

// noise returns the noise permission permutations for a given level. You can
// use this helper function when this level is not impactful.
// The returned role is the permutations including at least one example of
// positive, negative, and neutral permissions. It also includes the set of
// no additional permissions.
func noise(f noiseBits, lvls ...authztest.LevelGroup) *authztest.Role {
	rs := make([]authztest.Iterable, 0, len(lvls))
	for _, lvl := range lvls {
		sets := make([]authztest.Iterable, 0, bits.OnesCount8(uint8(f)))

		if flagMatch(noisePositive, f) {
			sets = append(sets, authztest.Union(lvl.Positive()[:1], nilSet))
		}
		if flagMatch(noiseNegative, f) {
			sets = append(sets, authztest.Union(lvl.Negative()[:1], nilSet))
		}
		if flagMatch(noiseAbstain, f) {
			sets = append(sets, authztest.Union(lvl.Abstain()[:1], nilSet))
		}

		rs = append(rs, authztest.NewRole(
			sets...,
		))
	}

	if len(rs) == 1 {
		return rs[0].(*authztest.Role)
	}
	return authztest.NewRole(rs...)
}
