# Group & Role Sync (Enterprise)

You can use groups and roles from your identity provider as the definitive source for Coder's user roles and groups.

## How it Works

1. **Configure OIDC**: Adjust your OIDC identity provider settings to transmit claims via the OIDC token or userinfo endpoint. These claims, usually labeled `groups` and `roles`, aren't sent by default in most OIDC clients.

1. **Configure Coder Server**: Coder can either:
   - A) Create new groups in Coder, or
   - B) Map claims to existing Coder groups/roles.

1. **Roles Sync on Login**: Upon user authentication, their associated groups and roles synchronize with Coder, using the identity provider as the reference.

## Group Sync (enterprise)

If your OpenID Connect provider supports group claims, you can configure Coder
to synchronize groups in your auth provider to groups within Coder.

To enable group sync, ensure that a groups claim is being sent. This is often `groups` or `memberOf`. Technically, a `roles` claim could be mapped to syncronize groups as Coder just expects an array of strings (e.g. `["Admin", "DevOps-Admin"`)

To check, [configure](../configure.md) the Coder server with the following environment variable to send verbose logs:

```sh
CODER_VERBOSE=true
```

Be sure to restart the server. When a user logs in with OIDC, you should see the following logs from the server.


```sh
[debu]  coderd.userauth: got oidc claims  trace=0x1b09780  span=0x1b09820  request_id=833f136a-2e6b-4df5-8ecb-1316c71a425a  source=id_token  claim_fields="[aio aud email exp groups iat idp iss name nbf oid preferred_username rh sub tid uti ver]"  blank=[]

[debu]  coderd.userauth: got oidc claims  trace=0x1b09780  span=0x1b09820  request_id=833f136a-2e6b-4df5-8ecb-1316c71a425a  source=userinfo  claim_fields="[email family_name given_name name picture sub]"  blank=[]

[debu]  coderd.userauth: got oidc claims  trace=0x1b09780  span=0x1b09820  request_id=833f136a-2e6b-4df5-8ecb-1316c71a425a  source=merged  claim_fields="[aio aud email exp family_name given_name groups iat idp iss name nbf oid picture preferred_username rh sub tid uti ver]"  blank=[]
```

> ℹ️ In this example, Coder is successfully getting the `groups` OIDC claim from the token and merging the claims from userinfo endpoint. See below for troubleshooting instructions.

### Enabling Group Sync

To enable group sync, you must tell Coder which claim to be used:

```sh
CODER_OIDC_GROUP_FIELD=groups
```

By default, Coder will only sync groups that match an existing group in Coder. However, there are two other options.

#### Automatically Create New Groups

To automatically create groups in Coder if they don't exist, set the following server value:

```console
CODER_OIDC_GROUP_AUTO_CREATE=true
```

#### Configuring Group Mapping

For cases when an OIDC provider only returns group IDs ([Azure AD][azure-gids])
or you want to have different group names in Coder than in your OIDC provider,
you can configure mapping between the two.

```console
CODER_OIDC_GROUP_MAPPING='{"myOIDCGroupID": "myCoderGroupName"}'
```

Below is an example mapping in the Coder Helm chart:

```yaml
coder:
  env:
    - name: CODER_OIDC_GROUP_MAPPING
      value: >
        {"myOIDCGroupID": "myCoderGroupName"}
```

### Filtering Group Sync

A basic regex filtering option on the Coder side is available. This is applied **after** the group mapping (`CODER_OIDC_GROUP_MAPPING`), meaning if the group is remapped, the remapped value is tested in the regex. This is useful if you want to filter out groups that do not match a certain pattern. For example, if you want to only allow groups that start with `my-group-` to be created, you can set the following environment variable.

```console
CODER_OIDC_GROUP_REGEX_FILTER="^my-group-.*$"
```

## Role Sync (enterprise)

If your OpenID Connect provider supports roles claims, you can configure Coder
to synchronize roles in your auth provider to deployment-wide roles within Coder.

Set the following in your Coder server [configuration](./configure.md).

```console
 # Depending on your identity provider configuration, you may need to explicitly request a "roles" scope
CODER_OIDC_SCOPES=openid,profile,email,roles

# The following fields are required for role sync:
CODER_OIDC_USER_ROLE_FIELD=roles
CODER_OIDC_USER_ROLE_MAPPING='{"TemplateAuthor":["template-admin","user-admin"]}'
```

> One role from your identity provider can be mapped to many roles in Coder (e.g. the example above maps to 2 roles in Coder.)

[azure-gids]: https://github.com/MicrosoftDocs/azure-docs/issues/59766#issuecomment-664387195

### Troubleshooting

Some common issues when enabling group and role sync.

#### No `groups` claim in the `got oidc claims` log

If you are not recieving the `groups` claim, refer to your identify provider documentation. In some cases, you will need to add the claim to your identity provider and request it via a scope in the OIDC config:

```sh
CODER_OIDC_SCOPES=openid,profile,email,groups
```

Here are some general steps:

1. Ensure the user is a part of a group in the IDP. If the user has 0 groups, no `groups` claim will be sent.
2. Check if another claim appears to be the correct claim with a different name. A common name is `memberOf` instead of `groups`. If this is present, update `CODER_OIDC_GROUP_FIELD=memberOf`.
3. Make sure the number of groups being sent is under the limit of the IDP. Some IDPs will return an error, while others will just omit the `groups` claim. A common solution is to create a filter on the identity provider that returns less than the limit for your IDP.
   - [Azure AD limit is 200, and omits groups if exceeded.](https://learn.microsoft.com/en-us/azure/active-directory/hybrid/connect/how-to-connect-fed-group-claims#options-for-applications-to-consume-group-information)
   - [Okta limit is 100, and returns an error if exceeded.](https://developer.okta.com/docs/reference/api/oidc/#scope-dependent-claims-not-always-returned)

#### Invalid Scope

If you see an error like the following, you may have an invalid scope.

```console
The application '<oidc_application>' asked for scope 'groups' that doesn't exist on the resource...
```

This can happen because the identity provider has a different name for the scope. For example, Azure AD uses `GroupMember.Read.All` instead of `groups`. You can find the correct scope name in the IDP's documentation. Some IDP's allow configuring the name of this scope.

The solution is to update the value of `CODER_OIDC_SCOPES` to the correct value for the identity provider.
