package authz
import future.keywords
# A great playground: https://play.openpolicyagent.org/
# Helpful cli commands to debug.
# opa eval --format=pretty 'data.authz.allow = true' -d policy.rego  -i input.json
# opa eval --partial --format=pretty 'data.authz.allow = true' -d policy.rego --unknowns input.object.owner --unknowns input.object.org_owner -i input.json



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

# number is a quick way to get a set of {true, false} and convert it to
#  -1: {false, true} or {false}
#   0: {}
#   1: {true}
number(set) = c {
	count(set) == 0
    c := 0
}

number(set) = c {
	false in set
    c := -1
}

number(set) = c {
	not false in set
	set[_]
    c := 1
}


default site = 0
site := num {
	allow := { x |
    	perm := input.subject.roles[_].site[_]
        perm.action in [input.action, "*"]
		perm.resource_type in [input.object.type, "*"]
        x := bool_flip(perm.negate)
    }
    num := number(allow)
}

org_members := { orgID |
	input.subject.roles[_].org[orgID]
}

default org = 0
org := num {
	orgPerms := { id: num |
		id := org_members[_]
		set := { x |
			perm := input.subject.roles[_].org[id][_]
			perm.action in [input.action, "*"]
			perm.resource_type in [input.object.type, "*"]
			x := bool_flip(perm.negate)
		}
		num := number(set)
	}

	num := orgPerms[input.object.org_owner]
}

# 'org_mem' is set to true if the user is an org member
# or if the object has no org.
org_mem := true {
	input.object.org_owner != ""
	input.object.org_owner in org_members
}

org_mem := true {
	input.object.org_owner == ""
}

default user = 0
user := num {
    input.object.owner != ""
    input.subject.id = input.object.owner
	allow := { x |
    	perm := input.subject.roles[_].user[_]
        perm.action in [input.action, "*"]
		perm.resource_type in [input.object.type, "*"]
        x := bool_flip(perm.negate)
    }
    num := number(allow)
}

default allow = false
# Site
allow {
	site = 1
}

# Org
allow {
	not site = -1
	org = 1
}

# User
allow {
	not site = -1
	not org = -1
	org_mem
	user = 1
}
