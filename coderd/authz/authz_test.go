package authz_test

import (
	"fmt"
	"github.com/coder/coder/coderd/authz/authztest"
	"math/bits"
	"testing"
)

var nilSet = authztest.Set{nil}

func Test_ExhaustiveAuthorize(t *testing.T) {
	all := authztest.GroupedPermissions(authztest.AllPermissions())
	variants := permissionVariants(all)
	for name, v := range variants {
		fmt.Printf("%s: %d\n", name, v.Size())
	}
}

func permissionVariants(all authztest.SetGroup) map[string]*authztest.Role {
	// an is any noise above the impactful set
	an := abstain
	// ln is any noise below the impactful set
	ln := positive | negative | abstain

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
		// TODO: Figure out cross org noise between org:* and org:mem
		// Org:*
		"O+": authztest.NewRole(
			noise(an, all.Wildcard(), all.Site()),
			pos(all.Org()),
			noise(ln, all.User()),
		),
		"O-": authztest.NewRole(
			noise(an, all.Wildcard(), all.Site()),
			neg(all.Org()),
			noise(ln, all.User()),
		),
		// Org:Mem
		"M+": authztest.NewRole(
			noise(an, all.Wildcard(), all.Site()),
			pos(all.OrgMem()),
			noise(ln, all.User()),
		),
		"M-": authztest.NewRole(
			noise(an, all.Wildcard(), all.Site()),
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
	}
}

func l() {
	//authztest.Levels
	//noise(an, all.Wildcard()),
	//	neg(all.Site()),
	//	noise(ln, all.Org(), all.User()),
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
	none noiseBits = 1 << iota
	positive
	negative
	abstain
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

		if flagMatch(positive, f) {
			sets = append(sets, authztest.Union(lvl.Positive()[:1], nilSet))
		}
		if flagMatch(negative, f) {
			sets = append(sets, authztest.Union(lvl.Negative()[:1], nilSet))
		}
		if flagMatch(abstain, f) {
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
