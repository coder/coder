package authztest

import "github.com/coder/coder/coderd/authz"

func Objects(pairs ...[]string) []authz.Resource {
	objs := make([]authz.Resource, 0, len(pairs))
	for _, p := range pairs {
		objs = append(objs,
			authz.ResourceType(PermObjectType).Owner(p[0]).Org(p[1]).AsID(PermObjectID),
		)
	}
	return objs
}
