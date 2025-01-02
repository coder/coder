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
number(set) := c if {
	count(set) == 0
	c := 0
}

number(set) := c if {
	false in set
	c := -1
}

number(set) := c if {
	not false in set
	set[_]
	c := 1
}

# site, org, and user rules are all similar. Each rule should return a number
# from [-1, 1]. The number corresponds to "negative", "abstain", and "positive"
# for the given level. See the 'allow' rules for how these numbers are used.
default site := 0

site := site_allow(input.subject.roles)

default scope_site := 0

scope_site := site_allow([input.subject.scope])

site_allow(roles) := num if {
	# allow is a set of boolean values without duplicates.
	allow := {x |
		# Iterate over all site permissions in all roles
		perm := roles[_].site[_]
		perm.action in [input.action, "*"]
		perm.resource_type in [input.object.type, "*"]

		# x is either 'true' or 'false' if a matching permission exists.
		x := bool_flip(perm.negate)
	}
	num := number(allow)
}

# org_members is the list of organizations the actor is apart of.
org_members := {orgID |
	input.subject.roles[_].org[orgID]
}

# org is the same as 'site' except we need to iterate over each organization
# that the actor is a member of.
default org := 0

org := org_allow(input.subject.roles)

default scope_org := 0

scope_org := org_allow([input.scope])

# org_allow_set is a helper function that iterates over all orgs that the actor
# is a member of. For each organization it sets the numerical allow value
# for the given object + action if the object is in the organization.
# The resulting value is a map that looks something like:
# {"10d03e62-7703-4df5-a358-4f76577d4e2f": 1, "5750d635-82e0-4681-bd44-815b18669d65": 1}
# The caller can use this output[<object.org_owner>] to get the final allow value.
#
# The reason we calculate this for all orgs, and not just the input.object.org_owner
# is that sometimes the input.object.org_owner is unknown. In those cases
# we have a list of org_ids that can we use in a SQL 'WHERE' clause.
org_allow_set(roles) := allow_set if {
	allow_set := {id: num |
		id := org_members[_]
		set := {x |
			perm := roles[_].org[id][_]
			perm.action in [input.action, "*"]
			perm.resource_type in [input.object.type, "*"]
			x := bool_flip(perm.negate)
		}
		num := number(set)
	}
}

org_allow(roles) := num if {
	# If the object has "any_org" set to true, then use the other
	# org_allow block.
	not input.object.any_org
	allow := org_allow_set(roles)

	# Return only the org value of the input's org.
	# The reason why we do not do this up front, is that we need to make sure
	# this policy compresses down to simple queries. One way to ensure this is
	# to keep unknown values out of comprehensions.
	# (https://www.openpolicyagent.org/docs/latest/policy-language/#comprehensions)
	num := allow[input.object.org_owner]
}

# This block states if "object.any_org" is set to true, then disregard the
# organization id the object is associated with. Instead, we check if the user
# can do the action on any organization.
# This is useful for UI elements when we want to conclude, "Can the user create
# a new template in any organization?"
# It is easier than iterating over every organization the user is apart of.
org_allow(roles) := num if {
	input.object.any_org # if this is false, this code block is not used
	allow := org_allow_set(roles)

	# allow is a map of {"<org_id>": <number>}. We only care about values
	# that are 1, and ignore the rest.
	num := number([
	keep |
		# for every value in the mapping
		value := allow[_]

		# only keep values > 0.
		# 1 = allow, 0 = abstain, -1 = deny
		# We only need 1 explicit allow to allow the action.
		# deny's and abstains are intentionally ignored.
		value > 0

		# result set is a set of [true,false,...]
		# which "number()" will convert to a number.
		keep := true
	])
}

# 'org_mem' is set to true if the user is an org member
# If 'any_org' is set to true, use the other block to determine org membership.
org_mem if {
	not input.object.any_org
	input.object.org_owner != ""
	input.object.org_owner in org_members
}

org_mem if {
	input.object.any_org
	count(org_members) > 0
}

org_ok if {
	org_mem
}

# If the object has no organization, then the user is also considered part of
# the non-existent org.
org_ok if {
	input.object.org_owner == ""
	not input.object.any_org
}

# User is the same as the site, except it only applies if the user owns the object and
# the user is apart of the org (if the object has an org).
default user := 0

user := user_allow(input.subject.roles)

default user_scope := 0

scope_user := user_allow([input.scope])

user_allow(roles) := num if {
	input.object.owner != ""
	input.subject.id = input.object.owner
	allow := {x |
		perm := roles[_].user[_]
		perm.action in [input.action, "*"]
		perm.resource_type in [input.object.type, "*"]
		x := bool_flip(perm.negate)
	}
	num := number(allow)
}

# Scope allow_list is a list of resource IDs explicitly allowed by the scope.
# If the list is '*', then all resources are allowed.
scope_allow_list if {
	"*" in input.subject.scope.allow_list
}

scope_allow_list if {
	# If the wildcard is listed in the allow_list, we do not care about the
	# object.id. This line is included to prevent partial compilations from
	# ever needing to include the object.id.
	not "*" in input.subject.scope.allow_list
	input.object.id in input.subject.scope.allow_list
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

role_allow if {
	not site = -1
	org = 1
}

role_allow if {
	not site = -1
	not org = -1

	# If we are not a member of an org, and the object has an org, then we are
	# not authorized. This is an "implied -1" for not being in the org.
	org_ok
	user = 1
}

scope_allow if {
	scope_allow_list
	scope_site = 1
}

scope_allow if {
	scope_allow_list
	not scope_site = -1
	scope_org = 1
}

scope_allow if {
	scope_allow_list
	not scope_site = -1
	not scope_org = -1

	# If we are not a member of an org, and the object has an org, then we are
	# not authorized. This is an "implied -1" for not being in the org.
	org_ok
	scope_user = 1
}

# ACL for users
acl_allow if {
	# Should you have to be a member of the org too?
	perms := input.object.acl_user_list[input.subject.id]

	# Either the input action or wildcard
	[input.action, "*"][_] in perms
}

# ACL for groups
acl_allow if {
	# If there is no organization owner, the object cannot be owned by an
	# org_scoped team.
	org_mem
	group := input.subject.groups[_]
	perms := input.object.acl_group_list[group]

	# Either the input action or wildcard
	[input.action, "*"][_] in perms
}

# ACL for 'all_users' special group
acl_allow if {
	org_mem
	perms := input.object.acl_group_list[input.object.org_owner]
	[input.action, "*"][_] in perms
}

###############
# Final Allow
# The role or the ACL must allow the action. Scopes can be used to limit,
# so scope_allow must always be true.

allow if {
	role_allow
	scope_allow
}

# ACL list must also have the scope_allow to pass
allow if {
	acl_allow
	scope_allow
}
