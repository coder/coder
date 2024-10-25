# IDP Sync (enterprise) (premium)

If your OpenID Connect provider supports group claims, you can configure Coder
to synchronize groups in your auth provider to groups within Coder. To enable
group sync, ensure that the `groups` claim is being sent by your OpenID
provider. You might need to request an additional
[scope](../../reference/cli/server.md#--oidc-scopes) or additional configuration
on the OpenID provider side.

If group sync is enabled, the user's groups will be controlled by the OIDC
provider. This means manual group additions/removals will be overwritten on the
next user login.

There are two ways you can configure group sync:

<div class="tabs">

## Server Flags

First, confirm that your OIDC provider is sending claims by logging in with OIDC
and visiting the following URL with an `Owner` account:

```text
https://[coder.example.com]/api/v2/debug/[your-username]/debug-link
```

You should see a field in either `id_token_claims`, `user_info_claims` or both
followed by a list of the user's OIDC groups in the response. This is the
[claim](https://openid.net/specs/openid-connect-core-1_0.html#Claims) sent by
the OIDC provider. See
[Troubleshooting](#troubleshooting-grouproleorganization-sync) to debug this.

> Depending on the OIDC provider, this claim may be named differently. Common
> ones include `groups`, `memberOf`, and `roles`.

Next configure the Coder server to read groups from the claim name with the
[OIDC group field](../../reference/cli/server.md#--oidc-group-field) server
flag:

```sh
# as an environment variable
CODER_OIDC_GROUP_FIELD=groups
```

```sh
# as a flag
--oidc-group-field groups
```

On login, users will automatically be assigned to groups that have matching
names in Coder and removed from groups that the user no longer belongs to.

For cases when an OIDC provider only returns group IDs ([Azure AD][azure-gids])
or you want to have different group names in Coder than in your OIDC provider,
you can configure mapping between the two with the
[OIDC group mapping](../../reference/cli/server.md#--oidc-group-mapping) server
flag.

```sh
# as an environment variable
CODER_OIDC_GROUP_MAPPING='{"myOIDCGroupID": "myCoderGroupName"}'
```

```sh
# as a flag
--oidc-group-mapping '{"myOIDCGroupID": "myCoderGroupName"}'
```

Below is an example mapping in the Coder Helm chart:

```yaml
coder:
  env:
    - name: CODER_OIDC_GROUP_MAPPING
      value: >
        {"myOIDCGroupID": "myCoderGroupName"}
```

From the example above, users that belong to the `myOIDCGroupID` group in your
OIDC provider will be added to the `myCoderGroupName` group in Coder.

[azure-gids]:
	https://github.com/MicrosoftDocs/azure-docs/issues/59766#issuecomment-664387195

## Runtime (Organizations)

> Note: You must have a Premium license with Organizations enabled to use this.
> [Contact your account team](https://coder.com/contact) for more details

For deployments with multiple [organizations](./organizations.md), you must
configure group sync at the organization level. In future Coder versions, you
will be able to configure this in the UI. For now, you must use CLI commands.

First confirm you have the [Coder CLI](../../install/index.md) installed and are
logged in with a user who is an Owner or Organization Admin role. Next, confirm
that your OIDC provider is sending a groups claim by logging in with OIDC and
visiting the following URL:

```text
https://[coder.example.com]/api/v2/debug/[your-username]/debug-link
```

You should see a field in either `id_token_claims`, `user_info_claims` or both
followed by a list of the user's OIDC groups in the response. This is the
[claim](https://openid.net/specs/openid-connect-core-1_0.html#Claims) sent by
the OIDC provider. See
[Troubleshooting](#troubleshooting-grouproleorganization-sync) to debug this.

> Depending on the OIDC provider, this claim may be named differently. Common
> ones include `groups`, `memberOf`, and `roles`.

To fetch the current group sync settings for an organization, run the following:

```sh
coder organizations settings show group-sync \
  --org <org-name> \
  > group-sync.json
```

The default for an organization looks like this:

```json
{
	"field": "",
	"mapping": null,
	"regex_filter": null,
	"auto_create_missing_groups": false
}
```

Below is an example that uses the `groups` claim and maps all groups prefixed by
`coder-` into Coder:

```json
{
	"field": "groups",
	"mapping": null,
	"regex_filter": "^coder-.*$",
	"auto_create_missing_groups": true
}
```

> Note: You much specify Coder group IDs instead of group names. The fastest way
> to find the ID for a corresponding group is by visiting
> `https://coder.example.com/api/v2/groups`.

Here is another example which maps `coder-admins` from the identity provider to
2 groups in Coder and `coder-users` from the identity provider to another group:

```json
{
	"field": "groups",
	"mapping": {
		"coder-admins": [
			"2ba2a4ff-ddfb-4493-b7cd-1aec2fa4c830",
			"93371154-150f-4b12-b5f0-261bb1326bb4"
		],
		"coder-users": ["2f4bde93-0179-4815-ba50-b757fb3d43dd"]
	},
	"regex_filter": null,
	"auto_create_missing_groups": false
}
```

To set these group sync settings, use the following command:

```sh
coder organizations settings set group-sync \
  --org <org-name> \
  < group-sync.json
```

Visit the Coder UI to confirm these changes:

![IDP Sync](../../images/admin/users/organizations/group-sync.png)

</div>

### Group allowlist

You can limit which groups from your identity provider can log in to Coder with
[CODER_OIDC_ALLOWED_GROUPS](https://coder.com/docs/cli/server#--oidc-allowed-groups).
Users who are not in a matching group will see the following error:

![Unauthorized group error](../../images/admin/group-allowlist.png)

## Role sync (enterprise) (premium)

If your OpenID Connect provider supports roles claims, you can configure Coder
to synchronize roles in your auth provider to roles within Coder.

There are 2 ways to do role sync. Server Flags assign site wide roles, and
runtime org role sync assigns organization roles

<div class="tabs">

## Server Flags

First, confirm that your OIDC provider is sending a roles claim by logging in
with OIDC and visiting the following URL with an `Owner` account:

```text
https://[coder.example.com]/api/v2/debug/[your-username]/debug-link
```

You should see a field in either `id_token_claims`, `user_info_claims` or both
followed by a list of the user's OIDC roles in the response. This is the
[claim](https://openid.net/specs/openid-connect-core-1_0.html#Claims) sent by
the OIDC provider. See
[Troubleshooting](#troubleshooting-grouproleorganization-sync) to debug this.

> Depending on the OIDC provider, this claim may be named differently.

Next configure the Coder server to read groups from the claim name with the
[OIDC role field](../../reference/cli/server.md#--oidc-user-role-field) server
flag:

Set the following in your Coder server [configuration](../setup/index.md).

```env
 # Depending on your identity provider configuration, you may need to explicitly request a "roles" scope
CODER_OIDC_SCOPES=openid,profile,email,roles

# The following fields are required for role sync:
CODER_OIDC_USER_ROLE_FIELD=roles
CODER_OIDC_USER_ROLE_MAPPING='{"TemplateAuthor":["template-admin","user-admin"]}'
```

> One role from your identity provider can be mapped to many roles in Coder
> (e.g. the example above maps to 2 roles in Coder.)

## Runtime (Organizations)

> Note: You must have a Premium license with Organizations enabled to use this.
> [Contact your account team](https://coder.com/contact) for more details

For deployments with multiple [organizations](./organizations.md), you can
configure role sync at the organization level. In future Coder versions, you
will be able to configure this in the UI. For now, you must use CLI commands.

First, confirm that your OIDC provider is sending a roles claim by logging in
with OIDC and visiting the following URL with an `Owner` account:

```text
https://[coder.example.com]/api/v2/debug/[your-username]/debug-link
```

You should see a field in either `id_token_claims`, `user_info_claims` or both
followed by a list of the user's OIDC roles in the response. This is the
[claim](https://openid.net/specs/openid-connect-core-1_0.html#Claims) sent by
the OIDC provider. See
[Troubleshooting](#troubleshooting-grouproleorganization-sync) to debug this.

> Depending on the OIDC provider, this claim may be named differently.

To fetch the current group sync settings for an organization, run the following:

```sh
coder organizations settings show role-sync \
  --org <org-name> \
  > role-sync.json
```

The default for an organization looks like this:

```json
{
	"field": "",
	"mapping": null
}
```

Below is an example that uses the `roles` claim and maps `coder-admins` from the
IDP as an `Organization Admin` and also maps to a custom `provisioner-admin`
role.

```json
{
	"field": "roles",
	"mapping": {
		"coder-admins": ["organization-admin"],
		"infra-admins": ["provisioner-admin"]
	}
}
```

> Note: Be sure to use the `name` field for each role, not the display name. Use
> `coder organization  roles show --org=<your-org>` to see roles for your
> organization.

To set these role sync settings, use the following command:

```sh
coder organizations settings set role-sync \
  --org <org-name> \
  < role-sync.json
```

Visit the Coder UI to confirm these changes:

![IDP Sync](../../images/admin/users/organizations/role-sync.png)

</div>

## Organization Sync (Premium)

> Note: In a future Coder release, this can be managed via the Coder UI instead
> of server flags.

If your OpenID Connect provider supports groups/role claims, you can configure
Coder to synchronize claims in your auth provider to organizations within Coder.

First, confirm that your OIDC provider is sending clainms by logging in with
OIDC and visiting the following URL with an `Owner` account:

```text
https://[coder.example.com]/api/v2/debug/[your-username]/debug-link
```

You should see a field in either `id_token_claims`, `user_info_claims` or both
followed by a list of the user's OIDC groups in the response. This is the
[claim](https://openid.net/specs/openid-connect-core-1_0.html#Claims) sent by
the OIDC provider. See
[Troubleshooting](#troubleshooting-grouproleorganization-sync) to debug this.

> Depending on the OIDC provider, this claim may be named differently. Common
> ones include `groups`, `memberOf`, and `roles`.

Next configure the Coder server to read groups from the claim name with the
[OIDC organization field](../../reference/cli/server.md#--oidc-organization-field)
server flag:

```sh
# as an environment variable
CODER_OIDC_ORGANIZATION_FIELD=groups
```

Next, fetch the corresponding organization IDs using the following endpoint:

```text
https://[coder.example.com]/api/v2/organizations
```

Set the following in your Coder server [configuration](../setup/index.md).

```env
CODER_OIDC_ORGANIZATION_MAPPING='{"data-scientists":["d8d9daef-e273-49ff-a832-11fe2b2d4ab1", "70be0908-61b5-4fb5-aba4-4dfb3a6c5787"]}'
```

> One claim value from your identity provider can be mapped to many
> organizations in Coder (e.g. the example above maps to 2 organizations in
> Coder.)

By default, all users are assigned to the default (first) organization. You can
disable that with:

```env
CODER_OIDC_ORGANIZATION_ASSIGN_DEFAULT=false
```

## Troubleshooting group/role/organization sync

Some common issues when enabling group/role sync.

### General guidelines

If you are running into issues with group/role sync, is best to view your Coder
server logs and enable
[verbose mode](../../reference/cli/index.md#-v---verbose). To reduce noise, you
can filter for only logs related to group/role sync:

```sh
CODER_VERBOSE=true
CODER_LOG_FILTER=".*userauth.*|.*groups returned.*"
```

Be sure to restart the server after changing these configuration values. Then,
attempt to log in, preferably with a user who has the `Owner` role.

The logs for a successful group sync look like this (human-readable):

```sh
[debu]  coderd.userauth: got oidc claims  request_id=49e86507-6842-4b0b-94d4-f245e62e49f3  source=id_token  claim_fields="[aio aud email exp groups iat idp iss name nbf oid preferred_username rh sub tid uti ver]"  blank=[]

[debu]  coderd.userauth: got oidc claims  request_id=49e86507-6842-4b0b-94d4-f245e62e49f3  source=userinfo  claim_fields="[email family_name given_name name picture sub]"  blank=[]

[debu]  coderd.userauth: got oidc claims  request_id=49e86507-6842-4b0b-94d4-f245e62e49f3  source=merged  claim_fields="[aio aud email exp family_name given_name groups iat idp iss name nbf oid picture preferred_username rh sub tid uti ver]"  blank=[]

[debu]  coderd: groups returned in oidc claims  request_id=49e86507-6842-4b0b-94d4-f245e62e49f3  email=ben@coder.com  username=ben  len=3  groups="[c8048e91-f5c3-47e5-9693-834de84034ad 66ad2cc3-a42f-4574-a281-40d1922e5b65 70b48175-107b-4ad8-b405-4d888a1c466f]"
```

To view the full claim, the Owner role can visit this endpoint on their Coder
deployment after logging in:

```sh
https://[coder.example.com]/api/v2/debug/[username]/debug-link
```

### User not being assigned / Group does not exist

If you want Coder to create groups that do not exist, you can set the following
environment variable. If you enable this, your OIDC provider might be sending
over many unnecessary groups. Use filtering options on the OIDC provider to
limit the groups sent over to prevent creating excess groups.

```env
# as an environment variable
CODER_OIDC_GROUP_AUTO_CREATE=true
```

```shell
# as a flag
--oidc-group-auto-create=true
```

A basic regex filtering option on the Coder side is available. This is applied
**after** the group mapping (`CODER_OIDC_GROUP_MAPPING`), meaning if the group
is remapped, the remapped value is tested in the regex. This is useful if you
want to filter out groups that do not match a certain pattern. For example, if
you want to only allow groups that start with `my-group-` to be created, you can
set the following environment variable.

```env
# as an environment variable
CODER_OIDC_GROUP_REGEX_FILTER="^my-group-.*$"
```

```shell
# as a flag
--oidc-group-regex-filter="^my-group-.*$"
```

### Invalid Scope

If you see an error like the following, you may have an invalid scope.

```console
The application '<oidc_application>' asked for scope 'groups' that doesn't exist on the resource...
```

This can happen because the identity provider has a different name for the
scope. For example, Azure AD uses `GroupMember.Read.All` instead of `groups`.
You can find the correct scope name in the IDP's documentation. Some IDP's allow
configuring the name of this scope.

The solution is to update the value of `CODER_OIDC_SCOPES` to the correct value
for the identity provider.

### No `group` claim in the `got oidc claims` log

Steps to troubleshoot.

1. Ensure the user is a part of a group in the IDP. If the user has 0 groups, no
   `groups` claim will be sent.
2. Check if another claim appears to be the correct claim with a different name.
   A common name is `memberOf` instead of `groups`. If this is present, update
   `CODER_OIDC_GROUP_FIELD=memberOf`.
3. Make sure the number of groups being sent is under the limit of the IDP. Some
   IDPs will return an error, while others will just omit the `groups` claim. A
   common solution is to create a filter on the identity provider that returns
   less than the limit for your IDP.
   - [Azure AD limit is 200, and omits groups if exceeded.](https://learn.microsoft.com/en-us/azure/active-directory/hybrid/connect/how-to-connect-fed-group-claims#options-for-applications-to-consume-group-information)
   - [Okta limit is 100, and returns an error if exceeded.](https://developer.okta.com/docs/reference/api/oidc/#scope-dependent-claims-not-always-returned)

## Provider-Specific Guides

Below are some details specific to individual OIDC providers.

### Active Directory Federation Services (ADFS)

> **Note:** Tested on ADFS 4.0, Windows Server 2019

1. In your Federation Server, create a new application group for Coder. Follow
   the steps as described
   [here.](https://learn.microsoft.com/en-us/windows-server/identity/ad-fs/development/msal/adfs-msal-web-app-web-api#app-registration-in-ad-fs)
   - **Server Application**: Note the Client ID.
   - **Configure Application Credentials**: Note the Client Secret.
   - **Configure Web API**: Set the Client ID as the relying party identifier.
   - **Application Permissions**: Allow access to the claims `openid`, `email`,
     `profile`, and `allatclaims`.
1. Visit your ADFS server's `/.well-known/openid-configuration` URL and note the
   value for `issuer`.
   > **Note:** This is usually of the form
   > `https://adfs.corp/adfs/.well-known/openid-configuration`
1. In Coder's configuration file (or Helm values as appropriate), set the
   following environment variables or their corresponding CLI arguments:

   - `CODER_OIDC_ISSUER_URL`: the `issuer` value from the previous step.
   - `CODER_OIDC_CLIENT_ID`: the Client ID from step 1.
   - `CODER_OIDC_CLIENT_SECRET`: the Client Secret from step 1.
   - `CODER_OIDC_AUTH_URL_PARAMS`: set to

     ```console
     {"resource":"$CLIENT_ID"}
     ```

     where `$CLIENT_ID` is the Client ID from step 1
     ([see here](https://learn.microsoft.com/en-us/windows-server/identity/ad-fs/overview/ad-fs-openid-connect-oauth-flows-scenarios#:~:text=scope%E2%80%AFopenid.-,resource,-optional)).
     This is required for the upstream OIDC provider to return the requested
     claims.

   - `CODER_OIDC_IGNORE_USERINFO`: Set to `true`.

1. Configure
   [Issuance Transform Rules](https://learn.microsoft.com/en-us/windows-server/identity/ad-fs/operations/create-a-rule-to-send-ldap-attributes-as-claims)
   on your federation server to send the following claims:

   - `preferred_username`: You can use e.g. "Display Name" as required.
   - `email`: You can use e.g. the LDAP attribute "E-Mail-Addresses" as
     required.
   - `email_verified`: Create a custom claim rule:

     ```console
     => issue(Type = "email_verified", Value = "true")
     ```

   - (Optional) If using Group Sync, send the required groups in the configured
     groups claim field. See [here](https://stackoverflow.com/a/55570286) for an
     example.

### Keycloak

The access_type parameter has two possible values: "online" and "offline." By
default, the value is set to "offline". This means that when a user
authenticates using OIDC, the application requests offline access to the user's
resources, including the ability to refresh access tokens without requiring the
user to reauthenticate.

To enable the `offline_access` scope, which allows for the refresh token
functionality, you need to add it to the list of requested scopes during the
authentication flow. Including the `offline_access` scope in the requested
scopes ensures that the user is granted the necessary permissions to obtain
refresh tokens.

By combining the `{"access_type":"offline"}` parameter in the OIDC Auth URL with
the `offline_access` scope, you can achieve the desired behavior of obtaining
refresh tokens for offline access to the user's resources.
