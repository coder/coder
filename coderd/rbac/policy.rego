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

# check_all_org_permissions creates a map from org ids to votes at each org
# level, for each org that the subject is a member of. It doesn't actually check
# if the object is in the same org; the callers do that:
#   - `org_ids_with_vote` picks the org ids with a given vote, and the known-org
#     rules test the object's org id for membership in that set, and
#   - the `any_org` clauses take the `max` vote.
# For example, the map will look something like this:
#
#   {"<org_id_a>": 1, "<org_id_b>": 0, "<org_id_c>": -1}
#
# We build the whole map, rather than just the vote for the object's org,
# because the org id _might_ be unknown during partial evaluation. To keep this
# policy compressible to simple queries we need to keep unknown values out of
# comprehensions.
#
# This is a helper function shared by the memoized vote-map rules below, so its
# per-call cost is paid at most once per (roles, key) combination.
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

# The vote maps below are complete rules with no arguments, so OPA evaluates
# each once per query and caches the result. A function is instead re-evaluated
# at every call site, so reading org votes through these rules keeps the policy
# from rebuilding the same vote map for the org, member, and scope paths on
# every authorization check.
role_org_votes := check_all_org_permissions(input.subject.roles, "org")

role_member_votes := check_all_org_permissions(input.subject.roles, "member")

scope_org_votes := check_all_org_permissions([input.subject.scope], "org")

scope_member_votes := check_all_org_permissions([input.subject.scope], "member")

# org_ids_with_vote returns the set of org ids in a vote map whose vote equals
# `wanted`. It depends only on the (fully known) vote map, never on the object's
# org id, so its result is ground during partial evaluation. The known-org
# rules test the object's org id for membership in this set, which lets the
# query compile to `organization_id = ANY(ARRAY[...])` instead of fanning out to
# one query per org.
org_ids_with_vote(votes, wanted) := {org_id |
	some org_id, vote in votes
	vote == wanted
}

default org := 0

# Known org: only ever votes to allow. See POLICY.md "Known-org asymmetry". The
# count guard keeps an empty allow set from emitting an unsatisfiable
# `org_owner in set()` residual during partial evaluation (OPA drops the whole
# branch instead).
org := 1 if {
	not input.object.any_org
	allow := org_ids_with_vote(role_org_votes, 1)
	count(allow) > 0
	input.object.org_owner in allow
}

# any_org: the highest org-level vote across every org. Unlike the known-org
# clause this can vote -1, which the allow rules honor via `not org = -1`.
org := vote if {
	input.object.any_org
	vote := max({v | some v in role_org_votes})
}

default scope_org := 0

scope_org := 1 if {
	not input.object.any_org
	allow := org_ids_with_vote(scope_org_votes, 1)
	count(allow) > 0
	input.object.org_owner in allow
}

scope_org := vote if {
	input.object.any_org
	vote := max({v | some v in scope_org_votes})
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
# The rules for this level mirror the organization level rules and read from the
# same memoized vote maps (`role_member_votes`, `scope_member_votes`,
# `role_org_votes`, `scope_org_votes`).

default org_member := 0

# Known org: allow when the subject owns the object and a member-level
# permission allows it. The allowed set folds in the org-level deny as a ground
# set difference (see POLICY.md "Known-org asymmetry"), and its value is fully
# known at partial-evaluation time, so the unknown org id appears in only one
# positive membership test and the decision never branches on it. The count
# guard keeps an empty set from emitting an unsatisfiable residual.
org_member := 1 if {
	# Object must be jointly owned by the user
	input.object.owner != ""
	input.subject.id = input.object.owner
	not input.object.any_org

	# Org-level deny is folded in as a ground set difference so a known org never
	# needs an org-level -1 vote (see POLICY.md "Known-org asymmetry").
	allowed := org_ids_with_vote(role_member_votes, 1) - org_ids_with_vote(role_org_votes, -1)
	count(allowed) > 0
	input.object.org_owner in allowed
}

# any_org: the highest member-level vote across every org. Org-level deny is
# applied by the `not org = -1` gate in the allow rules rather than folded in
# here, because `org` votes -1 in the any_org case.
org_member := vote if {
	# Object must be jointly owned by the user
	input.object.owner != ""
	input.subject.id = input.object.owner
	input.object.any_org
	vote := max({v | some v in role_member_votes})
}

default scope_org_member := 0

# Known org: like org_member, scoped to the subject's current scope.
scope_org_member := 1 if {
	# Object must be jointly owned by the user
	input.object.owner != ""
	input.subject.id = input.object.owner
	not input.object.any_org

	allowed := org_ids_with_vote(scope_member_votes, 1) - org_ids_with_vote(scope_org_votes, -1)
	count(allowed) > 0
	input.object.org_owner in allowed
}

scope_org_member := vote if {
	# Object must be jointly owned by the user
	input.object.owner != ""
	input.subject.id = input.object.owner
	input.object.any_org
	vote := max({v | some v in scope_member_votes})
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

	# For a known org this is always true: `org` never votes -1 for a known org,
	# because org-level deny is folded into `org_member`. It only blocks here in
	# the any_org case, where `org` can be -1 via `max`.
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

	# As with `not org = -1` above, this only blocks in the any_org case; for a
	# known org, scope org-level deny is folded into `scope_org_member`.
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
# AI agent designation rules                                                   #
#==============================================================================#

# An AI agent acts with its sponsoring human's roles, so input.subject.id is the
# sponsor and cannot identify the agent. The acting AI identity is carried
# separately in input.subject.ai_agent_id, and a workspace records the identity
# it is designated to in input.object.ai_agent_id. These rules confine an AI
# actor to workspaces designated to it, which is what keeps a chat agent out of
# its sponsor's ordinary workspaces and out of a sibling agent's workspaces.

# acting_ai_agent_id is defaulted rather than read directly. In rego,
# `not input.subject.missing_field = ""` evaluates to true, so reading the field
# directly would classify a subject whose field is absent as an AI agent and
# deny it every protected workspace action. Defaulting to the empty string makes
# an absent field mean "not an AI agent", which keeps humans unaffected if the
# input ever stops carrying the field.
default acting_ai_agent_id := ""

acting_ai_agent_id := input.subject.ai_agent_id

# Either marker is sufficient. Checking both means a half-populated subject
# fails closed instead of being treated as a human.
subject_is_ai_agent if {
	input.subject.type = "ai_agent"
}

subject_is_ai_agent if {
	acting_ai_agent_id != ""
}

# All three types address workspace rows.
is_workspace_object if {
	input.object.type in {"workspace", "workspace_dormant", "prebuilt_workspace"}
}

# Read supports workspace inventory for a chat, and create must be authorized
# before the workspace has an ID to designate. Creation is covered instead by
# the server designating every AI-created workspace before its first build.
#
# Agent lifecycle actions are daemon bookkeeping on agent rows, not access to
# the human workspace's runtime or credentials. A bound agent in an ordinary
# human workspace must be able to report startup, lifecycle, metadata, and app
# health. The subject's exact workspace allow list and the agent API's parent
# checks still constrain which rows it can create, update, or delete.
ai_designation_exempt_action if {
	input.action in {"read", "create", "create_agent", "update_agent", "delete_agent"}
}

# Defining the exempt actions rather than the protected ones means any workspace
# action added later is protected by default.
ai_workspace_action_requires_designation if {
	is_workspace_object
	not ai_designation_exempt_action
}

# Human and system subjects never evaluate designation.
ai_workspace_designation_allow if {
	not subject_is_ai_agent
}

# AI subjects may perform exempt actions, and are unaffected on every
# non-workspace resource.
ai_workspace_designation_allow if {
	subject_is_ai_agent
	not ai_workspace_action_requires_designation
}

# Protected actions require a populated acting identity and an exact match. An
# undesignated workspace carries the empty string and never matches, and a
# workspace designated to a different agent never matches. Unification is used
# on the object side because the object may be a partial value.
ai_workspace_designation_allow if {
	subject_is_ai_agent
	acting_ai_agent_id != ""
	input.object.ai_agent_id = acting_ai_agent_id
}

#==============================================================================#
# ACL rules                                                                    #
#==============================================================================#

# ACL for users
acl_allow if {
	# The subject must be a member of the object's organization for a
	# user ACL grant to apply.
	is_org_member
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

	# ...and, for an AI agent actor, allowed by workspace designation. This
	# always holds for human and system subjects.
	ai_workspace_designation_allow
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
