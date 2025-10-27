package authz

import rego.v1

# Check the POLICY.md file before editing this!
#
# https://play.openpolicyagent.org/
#

#==============================================================================#
# Site level rules                                                             #
#==============================================================================#

# Site level permissions allow the subject to use that permission on any object.
# For example, a site-level workspace.read permission means that the subject can
# see every workspace in the deployment, regardless of organization or owner.

default site := 0

site := check_site_permissions(input.subject.roles)

default scope_site := 0

scope_site := check_site_permissions([input.subject.scope])

check_site_permissions(roles) := vote if {
	allow := {is_allowed |
		# Iterate over all site permissions in all roles, and check which ones match
		# the action and object type.
		perm := roles[_].site[_]
		perm.action in [input.action, "*"]
		perm.resource_type in [input.object.type, "*"]

		# If a negative matching permission was found, then we vote to disallow it.
		# If the permission is not negative, then we vote to allow it.
		is_allowed := bool_flip(perm.negate)
	}
	vote := to_vote(allow)
}

#==============================================================================#
# User level rules                                                             #
#==============================================================================#

# User level rules apply to all objects owned by the subject which are not also
# owned by an org. Permissions for objects which are "jointly" owned by an org
# instead defer to the org member level rules.

default user := 0

user := check_user_permissions(input.subject.roles)

default scope_user := 0

scope_user := check_user_permissions([input.subject.scope])

check_user_permissions(roles) := vote if {
	# The object must be owned by the subject.
	input.subject.id = input.object.owner

	# If there is an org, use org_member permissions instead
	input.object.org_owner == ""
	not input.object.any_org

	allow := {is_allowed |
		# Iterate over all user permissions in all roles, and check which ones match
		# the action and object type.
		perm := roles[_].user[_]
		perm.action in [input.action, "*"]
		perm.resource_type in [input.object.type, "*"]

		# If a negative matching permission was found, then we vote to disallow it.
		# If the permission is not negative, then we vote to allow it.
		is_allowed := bool_flip(perm.negate)
	}
	vote := to_vote(allow)
}

#==============================================================================#
# Org level rules                                                              #
#==============================================================================#

# Org level permissions are similar to `site`, except we need to iterate over
# each organization that the subject is a member of, and check against the
# organization that the object belongs to.
# For example, an organization-level workspace.read permission means that the
# subject can see every workspace in the organization, regardless of owner.

# org_memberships is the set of organizations the subject is apart of.
org_memberships := {org_id |
	input.subject.roles[_].by_org_id[org_id]
}

# TODO: Should there be a scope_org_memberships too? Without it, the membership
# is determined by the user's roles, not their scope permissions.
#
# If an owner (who is not an org member) has an org scope, that org scope will
# fail to return '1', since we assume all non-members return '-1' for org level
# permissions. Adding a second set of org memberships might affect the partial
# evaluation. This is being left until org scopes are used.

default org := 0

org := check_org_permissions(input.subject.roles, "org")

default scope_org := 0

scope_org := check_org_permissions([input.subject.scope], "org")

# check_all_org_permissions creates a map from org ids to votes at each org
# level, for each org that the subject is a member of. It doesn't actually check
# if the object is in the same org. Instead we look up the correct vote from
# this map based on the object's org id in `check_org_permissions`.
# For example, the `org_map` will look something like this:
#
#   {"<org_id_a>": 1, "<org_id_b>": 0, "<org_id_c>": -1}
#
# The caller then uses `output[input.object.org_owner]` to get the correct vote.
#
# We have to create this map, rather than just getting the vote of the object's
# org id because the org id _might_ be unknown. In order to make sure that this
# policy compresses down to simple queries we need to keep unknown values out of
# comprehensions.
check_all_org_permissions(roles, key) := {org_id: vote |
	org_id := org_memberships[_]
	allow := {is_allowed |
		# Iterate over all site permissions in all roles, and check which ones match
		# the action and object type.
		perm := roles[_].by_org_id[org_id][key][_]
		perm.action in [input.action, "*"]
		perm.resource_type in [input.object.type, "*"]

		# If a negative matching permission was found, then we vote to disallow it.
		# If the permission is not negative, then we vote to allow it.
		is_allowed := bool_flip(perm.negate)
	}
	vote := to_vote(allow)
}

# This check handles the case where the org id is known.
check_org_permissions(roles, key) := vote if {
	# Disallow setting any_org at the same time as an org id.
	not input.object.any_org

	allow_map := check_all_org_permissions(roles, key)

	# Return only the vote of the object's org.
	vote := allow_map[input.object.org_owner]
}

# This check handles the case where we want to know if the user has the
# appropriate permission for any organization, without needing to know which.
# This is used in several places in the UI to determine if certain parts of the
# app should be accessible.
# For example, can the user create a new template in any organization? If yes,
# then we should show the "New template" button.
check_org_permissions(roles, key) := vote if {
	# Require `any_org` to be set
	input.object.any_org

	allow_map := check_all_org_permissions(roles, key)

	# Since we're checking if the subject has the permission in _any_ org, we're
	# essentially trying to find the highest vote from any org.
	vote := max({vote |
		some vote in allow_map
	})
}

# is_org_member checks if the subject belong to the same organization as the
# object.
is_org_member if {
	not input.object.any_org
	input.object.org_owner != ""
	input.object.org_owner in org_memberships
}

# ...if 'any_org' is set to true, we check if the subject is a member of any
# org.
is_org_member if {
	input.object.any_org
	count(org_memberships) > 0
}

#==============================================================================#
# Org member level rules                                                       #
#==============================================================================#

# Org member level permissions apply to all objects owned by the subject _and_
# the corresponding org. Permissions for objects which are not owned by an
# organization instead defer to the user level rules.
#
# The rules for this level are very similar to the rules for the organization
# level, and so we reuse the `check_org_permissions` function from those rules.

default org_member := 0

org_member := vote if {
	# Object must be jointly owned by the user
	input.object.owner != ""
	input.subject.id = input.object.owner
	vote := check_org_permissions(input.subject.roles, "member")
}

default scope_org_member := 0

scope_org_member := vote if {
	# Object must be jointly owned by the user
	input.object.owner != ""
	input.subject.id = input.object.owner
	vote := check_org_permissions([input.subject.scope], "member")
}

#==============================================================================#
# Role rules                                                                   #
#==============================================================================#

# role_allow specifies all of the conditions under which a role can grant
# permission. These rules intentionally use the "unification" operator rather
# than the equality and inequality operators, because those operators do not
# work on partial values.
# https://www.openpolicyagent.org/docs/policy-language#unification-

# Site level authorization
role_allow if {
	site = 1
}

# User level authorization
role_allow if {
	not site = -1

	user = 1
}

# Org level authorization
role_allow if {
	not site = -1

	org = 1
}

# Org member authorization
role_allow if {
	not site = -1
	not org = -1

	org_member = 1
}

#==============================================================================#
# Scope rules                                                                  #
#==============================================================================#

# scope_allow specifies all of the conditions under which a scope can grant
# permission. These rules intentionally use the "unification" (=) operator
# rather than the equality (==) and inequality (!=) operators, because those
# operators do not work on partial values.
# https://www.openpolicyagent.org/docs/policy-language#unification-

# Site level scope enforcement
scope_allow if {
	object_is_included_in_scope_allow_list
	scope_site = 1
}

# User level scope enforcement
scope_allow if {
	# User scope permissions must be allowed by the scope, and not denied
	# by the site. The object *must not* be owned by an organization.
	object_is_included_in_scope_allow_list
	not scope_site = -1

	scope_user = 1
}

# Org level scope enforcement
scope_allow if {
	# Org member scope permissions must be allowed by the scope, and not denied
	# by the site. The object *must* be owned by an organization.
	object_is_included_in_scope_allow_list
	not scope_site = -1

	scope_org = 1
}

# Org member level scope enforcement
scope_allow if {
	# Org member scope permissions must be allowed by the scope, and not denied
	# by the site or org. The object *must* be owned by an organization.
	object_is_included_in_scope_allow_list
	not scope_site = -1
	not scope_org = -1

	scope_org_member = 1
}

# If *.* is allowed, then all objects are in scope.
object_is_included_in_scope_allow_list if {
	{"type": "*", "id": "*"} in input.subject.scope.allow_list
}

# If <type>.* is allowed, then all objects of that type are in scope.
object_is_included_in_scope_allow_list if {
	{"type": input.object.type, "id": "*"} in input.subject.scope.allow_list
}

# Check if the object type and ID match one of the allow list entries.
object_is_included_in_scope_allow_list if {
	# Check that the wildcard rules do not apply. This prevents partial inputs
	# from needing to include `input.object.id`.
	not {"type": "*", "id": "*"} in input.subject.scope.allow_list
	not {"type": input.object.type, "id": "*"} in input.subject.scope.allow_list

	# Check which IDs from the allow list match the object type
	allowed_ids_for_object_type := {it.id |
		some it in input.subject.scope.allow_list
		it.type in [input.object.type, "*"]
	}

	# Check if the input object ID is in the set of allowed IDs for the same
	# object type. We do this at the end to keep `input.object.id` out of the
	# comprehension because it might be unknown.
	input.object.id in allowed_ids_for_object_type
}

#==============================================================================#
# ACL rules                                                                    #
#==============================================================================#

# ACL for users
acl_allow if {
	# TODO: Should you have to be a member of the org too?
	perms := input.object.acl_user_list[input.subject.id]

	# Check if either the action or * is allowed
	some action in [input.action, "*"]
	action in perms
}

# ACL for groups
acl_allow if {
	# If there is no organization owner, the object cannot be owned by an
	# org-scoped group.
	is_org_member
	some group in input.subject.groups
	perms := input.object.acl_group_list[group]

	# Check if either the action or * is allowed
	some action in [input.action, "*"]
	action in perms
}

# ACL for the special "Everyone" groups
acl_allow if {
	# If there is no organization owner, the object cannot be owned by an
	# org-scoped group.
	is_org_member
	perms := input.object.acl_group_list[input.object.org_owner]

	# Check if either the action or * is allowed
	some action in [input.action, "*"]
	action in perms
}

#==============================================================================#
# Allow                                                                        #
#==============================================================================#

# The `allow` block is quite simple. Any check that voted no will cascade down.
# Authorization looks for any `allow` statement that is true. Multiple can be
# true! Note that the absence of `allow` means "unauthorized". An explicit
# `"allow": true` is required.
#
# We check both the subject's permissions (given by their roles or by ACL) and
# the subject's scope. (The default scope is "*:*", allowing all actions.) Both
# a permission check (either from roles or ACL) and the scope check must vote to
# allow or the action is not authorized.

# A subject can be given permission by a role
permission_allow if role_allow

# A subject can be given permission by ACL
permission_allow if acl_allow

allow if {
	# Must be allowed by the subject's permissions
	permission_allow

	# ...and allowed by the scope
	scope_allow
}

#==============================================================================#
# Utilities                                                                    #
#==============================================================================#

# bool_flip returns the logical negation of a boolean value. You can't do
# 'x := not false', but you can do 'x := bool_flip(false)'
bool_flip(b) := false if {
	b
}

bool_flip(b) if {
	not b
}

# to_vote gives you a voting value from a set or list of booleans.
#   {false,..} => deny (-1)
#   {}         => abstain (0)
#   {true}     => allow (1)

# Any set which contains a `false` should be considered a vote to deny.
to_vote(set) := -1 if {
	false in set
}

# A set which is empty should be considered abstaining.
to_vote(set) := 0 if {
	count(set) == 0
}

# A set which only contains true should be considered a vote to allow.
to_vote(set) := 1 if {
	not false in set
	true in set
}
