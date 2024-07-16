# Licensing

Licensing in Coderd defines what features are allowed to be used by a given deployment. Without a license, or with a license that grants 0 features, Coderd will refuse to execute code paths for some features. These features are typically gated with a middleware that checks the license before allowing the request to proceed.


## Terms

- **Feature**: A specific functionality that Coderd provides, such as external provisioners. Features are defined in the `Feature` enum in [`codersdk/deployment.go`](https://github.com/coder/coder/blob/main/codersdk/deployment.go#L36-L60). A feature can be "entitled", "grace period", or "not entitled". Additionally, a feature can be "disabled" that prevents usage of the feature even if the deployment is entitled to it. Disabled features are a configuration option by the deployment operator.
  - **Entitled**: The feature is allowed to be used by the deployment.
  - **Grace Period**: The feature is allowed to be used by the deployment, but the license is expired. There is a grace period before the feature is disabled.
  - **Not Entitled**: The feature is not allowed to be used by the deployment. Either by expiration, or by not being included in the license.
- **License**: A signed JWT that lists the features that are allowed to be used by a given deployment. A license can have extra properties like, `IsTrial`, `DeploymentIDs`, etc that can be used to further define usage of the license.
- **Entitlements**: A parsed set of licenses, yes you can have more than 1 license on a deployment! Entitlements will enumerate all features that are allowed to be used.
