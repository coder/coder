package authz
import future.keywords.in
import future.keywords.every

# A great playground: https://play.openpolicyagent.org/
# TODO: Add debug instructions to do in the cli. Running really short on time, the
#   playground is sufficient for now imo. In the future we can provide a tidy bash
#   script for running this against predefined input.

# bool_flip lets you assign a value to an inverted bool.
# You cannot do 'x := !false', but you can do 'x := bool_flip(false)'
bool_flip(b) = flipped {
    b
    flipped = false
}

bool_flip(b) = flipped {
    not b
    flipped = true
}

# perms_grant returns a set of boolean values {true, false}.
# True means a positive permission in the set, false is a negative permission.
# It will only return `bool_flip(perm.negate)` for permissions that affect a given
# resource_type, resource_id, and action.
# The empty set is returned if no relevant permissions are found.
perms_grant(permissions) = grants {
    # If there are no permissions, this value is the empty set {}.
    grants := { x |
        # All permissions ...
        perm := permissions[_]
        # Such that the permission action, type, and resource_id matches
        perm.action in [input.action, "*"]
        perm.resource_type in [input.object.type, "*"]
        perm.resource_id in [input.object.id, "*"]
        x := bool_flip(perm.negate)
    }
}

# Site & User are both very simple. We default both to the empty set '{}'. If no permissions are present, then the
# result is the default value.
default site = {}
site = grant {
    # Boolean set for all site wide permissions.
    grant = { v | # Use set comprehension to remove duplicate values
        # For each role, grab the site permission.
        # Find the grants on this permission list.
        v = perms_grant(input.subject.roles[_].site)[_]
    }
}

default user = {}
user = grant {
    # Only apply user permissions if the user owns the resource
    input.object.owner != ""
    input.object.owner == input.subject.id
    grant = { v |
        # For each role, grab the user permissions.
        # Find the grants on this permission list.
        v = perms_grant(input.subject.roles[_].user)[_]
    }
}

# Organizations are more complex. If the user has no roles that specifically indicate the org_id of the object,
# then we want to block the action. This is because that means the user is not a member of the org.
# A non-member cannot access any org resources.

# org_member returns the set of permissions associated with a user if the user is a member of the
# organization
org_member = grant {
    input.object.org_owner != ""
    grant = { v |
        v = perms_grant(input.subject.roles[_].org[input.object.org_owner])[_]
    }
}

# If a user is not part of an organization, 'org_non_member' is set to true
org_non_member {
    input.object.org_owner != ""
    # Identify if the user is in the org
    roles := input.subject.roles
    every role in roles {
        not role.org[input.object.org_owner]
    }
}

# org is two rules that equate to the following
#	if org_non_member { return {false} }
#	else { org_member }
#
# It is important both rules cannot be true, as the `org` rules cannot produce multiple outputs.
default org = {}
org = set {
    # We have to do !org_non_member because rego rules must evaluate to 'true'
    # to have a value set.
    # So we do "not not-org-member" which means "subject is in org"
    not org_non_member
    set = org_member
}

org = set {
    org_non_member
    set = {false}
}

# The allow block is quite simple. Any set with `false` cascades down in levels.
# Authorization looks for any `allow` statement that is true. Multiple can be true!
# Note that the absence of `allow` means "unauthorized".
# An explicit `"allow": true` is required.

# site allow
allow {
    # No site wide deny
    not false in site
    # And all permissions are positive
    site[_]
}

# OR

# org allow
allow {
    # No site or org deny
    not false in site
    not false in org
    # And all permissions are positive
    org[_]
}

# OR

# user allow
allow {
    # No site, org, or user deny
    not false in site
    not false in org
    not false in user
    # And all permissions are positive
    user[_]
}