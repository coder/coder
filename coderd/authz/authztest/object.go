package authztest

import "github.com/coder/coder/coderd/authz"

func Objects(pairs ...[]string) []authz.Object {
	objs := make([]authz.Object, 0, len(pairs))
	for _, p := range pairs {
		objs = append(objs, authz.Object{
			ObjectID:   PermObjectID,
			OwnerID:    p[0],
			OrgOwnerID: p[1],
			ObjectType: PermObjectType,
		})
	}
	return objs
}
