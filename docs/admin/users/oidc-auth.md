# OpenID Connect

The following steps through how to integrate any OpenID Connect provider (Okta,
Active Directory, etc.) to Coder.

## Step 1: Set Redirect URI with your OIDC provider

Your OIDC provider will ask you for the following parameter:

- **Redirect URI**: Set to `https://coder.domain.com/api/v2/users/oidc/callback`

## Step 2: Configure Coder with the OpenID Connect credentials

Set the following environment variables on your Coder deployment and restart Coder:

```env
CODER_OIDC_ISSUER_URL="https://issuer.corp.com"
CODER_OIDC_EMAIL_DOMAIN="your-domain-1,your-domain-2"
CODER_OIDC_CLIENT_ID="533...des"
CODER_OIDC_CLIENT_SECRET="G0CSP...7qSM"
```

## OIDC Claims

When a user logs in for the first time via OIDC, Coder will merge both the
claims from the ID token and the claims obtained from hitting the upstream
provider's `userinfo` endpoint, and use the resulting data as a basis for
creating a new user or looking up an existing user.

To troubleshoot claims, set `CODER_VERBOSE=true` and follow the logs while
signing in via OIDC as a new user. Coder will log the claim fields returned by
the upstream identity provider in a message containing the string
`got oidc claims`, as well as the user info returned.

> **Note:** If you need to ensure that Coder only uses information from the ID
> token and does not hit the UserInfo endpoint, you can set the configuration
> option `CODER_OIDC_IGNORE_USERINFO=true`.

### Email Addresses

By default, Coder will look for the OIDC claim named `email` and use that value
for the newly created user's email address.

If your upstream identity provider users a different claim, you can set
`CODER_OIDC_EMAIL_FIELD` to the desired claim.

> **Note** If this field is not present, Coder will attempt to use the claim
> field configured for `username` as an email address. If this field is not a
> valid email address, OIDC logins will fail.

### Email Address Verification

Coder requires all OIDC email addresses to be verified by default. If the
`email_verified` claim is present in the token response from the identity
provider, Coder will validate that its value is `true`. If needed, you can
disable this behavior with the following setting:

```env
CODER_OIDC_IGNORE_EMAIL_VERIFIED=true
```

> **Note:** This will cause Coder to implicitly treat all OIDC emails as
> "verified", regardless of what the upstream identity provider says.

### Usernames

When a new user logs in via OIDC, Coder will by default use the value of the
claim field named `preferred_username` as the the username.

If your upstream identity provider uses a different claim, you can set
`CODER_OIDC_USERNAME_FIELD` to the desired claim.

> **Note:** If this claim is empty, the email address will be stripped of the
> domain, and become the username (e.g. `example@coder.com` becomes `example`).
> To avoid conflicts, Coder may also append a random word to the resulting
> username.

## OIDC Login Customization

If you'd like to change the OpenID Connect button text and/or icon, you can
configure them like so:

```env
CODER_OIDC_SIGN_IN_TEXT="Sign in with Gitea"
CODER_OIDC_ICON_URL=https://gitea.io/images/gitea.png
```

To change the icon and text above the OpenID Connect button, see application
name and logo url in [appearance](../setup/appearance.md) settings.

## Disable Built-in Authentication

To remove email and password login, set the following environment variable on
your Coder deployment:

```env
CODER_DISABLE_PASSWORD_AUTH=true
```

## SCIM

> [!NOTE]
> SCIM is an Enterprise and Premium feature.
> [Learn more](https://coder.com/pricing#compare-plans).

Coder supports user provisioning and deprovisioning via SCIM 2.0 with header
authentication. Upon deactivation, users are
[suspended](./index.md#suspend-a-user) and are not deleted.
[Configure](../setup/index.md) your SCIM application with an auth key and supply
it the Coder server.

```env
CODER_SCIM_AUTH_HEADER="your-api-key"
```

## TLS

If your OpenID Connect provider requires client TLS certificates for
authentication, you can configure them like so:

```env
CODER_TLS_CLIENT_CERT_FILE=/path/to/cert.pem
CODER_TLS_CLIENT_KEY_FILE=/path/to/key.pem
```

### Next steps

- [Group Sync](./idp-sync.md)
- [Groups & Roles](./groups-roles.md)
