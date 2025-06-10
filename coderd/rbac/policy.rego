package authz

import rego.v1

# A great playground: https://play.openpolicyagent.org/
# Helpful cli commands to debug.
# opa eval --format=pretty 'data.authz.allow' -d policy.rego  -i input.json
# opa eval --partial --format=pretty 'data.authz.allow' -d policy.rego --unknowns input.object.owner --unknowns input.object.org_owner --unknowns input.object.acl_user_list --unknowns input.object.acl_group_list -i input.json

#
# This policy is specifically constructed to compress to a set of queries if the
# object's 'owner' and 'org_owner' fields are unknown. There is no specific set
# of rules that will guarantee that this policy has this property. However, there
# are some tricks. A unit test will enforce this property, so any edits that pass
# the unit test will be ok.
#
# Tricks: (It's hard to really explain this, fiddling is required)
# 1. Do not use unknown fields in any comprehension or iteration.
# 2. Use the unknown fields as minimally as possible.
# 3. Avoid making code branches based on the value of the unknown field.
#    Unknown values are like a "set" of possible values.
#    (This is why rule 1 usually breaks things)
#    For example:
#    In the org section, we calculate the 'allow' number for all orgs, rather
#    than just the input.object.org_owner. This is because if the org_owner
#    changes, then we don't need to recompute any 'allow' sets. We already have
#    the 'allow' for the changed value. So the answer is in a lookup table.
#    The final statement 'num := allow[input.object.org_owner]' does not have
#    different code branches based on the org_owner. 'num's value does, but
#    that is the whole point of partial evaluation.

# bool_flip lets you assign a value to an inverted bool.
# You cannot do 'x := !false', but you can do 'x := bool_flip(false)'
bool_flip(b) := flipped if {
	b
	flipped = false
}

bool_flip(b) := flipped if {
	not b
	flipped = true
}

# number is a quick way to get a set of {true, false} and convert it to
#  -1: {false, true} or {false}
#   0: {}
#   1: {true}
# Return 0 if the set is empty (no matching permissions)
number(set) := c if {
	count(set) == 0
	c := 0
}

# Return -1 if the set contains any 'false' (i.e., an explicit deny)
number(set) := c if {
	false in set
	c := -1
}

# Return 1 if the set is non-empty and contains no 'false' (i.e., only allows)
number(set) := c if {
	not false in set
	set[_]
	c := 1
}


prebuild_workspace_type := "prebuilt_workspace"
default_object_set := [input.object.type, "*"]
is_prebuild_workspace := true if {
	input.object.type = "workspace"
	input.object.owner = "c42fdf75-3097-471c-8c33-fb52454d81c0"
}

# site, org, and user rules are all similar. Each rule should return a number
# from [-1, 1]. The number corresponds to "negative", "abstain", and "positive"
# for the given level. See the 'allow' rules for how these numbers are used.
default site := 0

# test := number({1, 1, -1})
prebuild_object_set := ["*", prebuild_workspace_type]

default scope_site := 0

#site := site_allow(input.subject.roles, global_object_set)
#scope_site := site_allow([input.subject.scope], global_object_set)

site_all := site_all_allow(input.subject.roles, [input.subject.scope], global_object_set)

site_all_allow(subject_roles, subject_scope, local_object_set) := num if {
	# allow is a set of boolean values without duplicates.
	roles_allow := {x |
		# Iterate over all site permissions in all roles
		perm := subject_roles[_].site[_]
		perm.action in [input.action, "*"]
		perm.resource_type in prebuild_object_set

		# x is either 'true' or 'false' if a matching permission exists.
		x := bool_flip(perm.negate)
	}
	scope_allow := {y |
		# Iterate over all site permissions in all roles
		perm := subject_scope[_].site[_]
		perm.action in [input.action, "*"]
		perm.resource_type in prebuild_object_set

		# y is either 'true' or 'false' if a matching permission exists.
		y := bool_flip(perm.negate)
	}
	num := number({roles_allow, scope_allow})
}

# The allow block is quite simple. Any set with `-1` cascades down in levels.
# Authorization looks for any `allow` statement that is true. Multiple can be true!
# Note that the absence of `allow` means "unauthorized".
# An explicit `"allow": true` is required.
#
# Scope is also applied. The default scope is "wildcard:wildcard" allowing
# all actions. If the scope is not "1", then the action is not authorized.
#
#
# Allow query:
#	 data.authz.role_allow = true data.authz.scope_allow = true

role_allow if {
	site = 1
}

scope_allow if {
	scope_site = 1
}

###############
# Final Allow
# The role or the ACL must allow the action. Scopes can be used to limit,
# so scope_allow must always be true.

global_object_set := default_object_set if {
 	not is_prebuild_workspace
}

global_object_set := [input.object.type, "*", prebuild_workspace_type] if {
 	is_prebuild_workspace
}

allow if {
	site_all
}
