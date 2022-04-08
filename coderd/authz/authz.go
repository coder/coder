package authz

import "golang.org/x/xerrors"

var ErrUnauthorized = xerrors.New("unauthorized")

// TODO: Implement Authorize. This will be implmented in mainly rego.
func Authorize(subj Subject, obj Resource, action Action) error {
	// TODO: Expand subject roles into their permissions as appropriate. Apply scopes.
	var _, _, _ = subj, obj, action
	roles, err := subj.GetRoles()
	if err != nil {
		return ErrUnauthorized
	}

	// Merge before we send to rego to optimize the json payload.
	// TODO: Benchmark the rego, it might be ok to just send everything and let
	//		rego do the merges. The number of roles will be small, so it might not
	//		matter. This code exists just to show how you can merge the roles
	//		into a single one for evaluation if need be.
	//		If done in rego, the roles will not be merged, and just walked over
	//		1 by 1.
	var merged Role
	for _, r := range roles {
		// Site, Org, and User permissions exist on every role. Pull out only the permissions that
		// are relevant to the object.

		merged.Site = append(merged.Site, r.Site...)
		// Only grab user roles if the resource is owned by a user.
		// These roles only apply if the subject is said owner.
		if obj.OwnerID() != "" && obj.OwnerID() == subj.ID() {
			merged.User = append(merged.User, r.User...)
		}

		// Grab org roles if the resource is owned by a given organization.
		if obj.OrgOwnerID() != "" {
			orgID := obj.OrgOwnerID()
			if v, ok := r.Org[orgID]; ok {
				merged.Org[orgID] = append(merged.Org[orgID], v...)
			}
		}
	}

	// TODO: Send to rego policy evaluation.
	return nil
}
