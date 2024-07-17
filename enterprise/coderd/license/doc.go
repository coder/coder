// Package license provides the license parsing and validation logic for Coderd.
// Licensing in Coderd defines what features are allowed to be used in a
// given deployment. Without a license, or with a license that grants 0 features,
// Coderd will refuse to execute some feature code paths. These features are
// typically gated with a middleware that checks the license before allowing
// the http request to proceed.
//
// Terms:
// - FeatureName: A specific functionality that Coderd provides, such as
//                external provisioners.
//
// - Feature: Entitlement definition for a FeatureName. A feature can be:
//				- "entitled": The feature is allowed to be used by the deployment.
//				- "grace period": The feature is allowed to be used by the deployment,
//                                but the license is expired. There is a grace period
//                                before the feature is disabled.
//				- "not entitled": The deployment is not allowed to use the feature.
//	                              Either by expiration, or by not being included
//	                              in the license.
//            A feature can also be "disabled" that prevents usage of the feature
//            even if entitled. This is usually a deployment configuration option.
//
// - License: A signed JWT that lists the features that are allowed to be used by
//            a given deployment. A license can have extra properties like,
//            `IsTrial`, `DeploymentIDs`, etc that can be used to further define
//            usage of the license.
/**/
// - Entitlements: A parsed set of licenses. Yes you can have more than 1 license
//                 on a deployment! Entitlements will enumerate all features that
//                 are allowed to be used.
//
package license
