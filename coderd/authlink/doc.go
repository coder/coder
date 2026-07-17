// Package authlink provides analysis and repair utilities for OIDC user link
// records stored in the user_links table.
//
// When an OIDC provider is changed, the issuer (and possibly subject) in the
// linked_id column changes. Because linked_id is composed as "issuer||subject",
// existing users get locked out with "Account already linked" errors. The
// functions in this package let an administrator inspect which links are
// affected and reset the mismatched ones so users can re-authenticate under the
// new provider.
package authlink
