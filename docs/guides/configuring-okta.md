# Configuring Custom Claims/Scopes with Okta for group/role sync

### Author: Steven Masely

> Okta is an identity provider that can be used for OpenID Connect (OIDC) Single Sign On (SSO) on Coder.

To configure custom claims in Okta to support syncing roles and groups with Coder, you must first have setup an Okta application with [OIDC working with Coder](https://coder.com/docs/v2/latest/admin/auth#openid-connect). From here, we will add additional claims for Coder to use for syncing groups and roles.

You may use a hybrid of the following approaches.
