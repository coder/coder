# OIDC Authentication

With OIDC authentication, users can log in to your Coder deployment using your identify provider (e.g. KeyCloak, Okta, PingFederate, Azure AD, etc.)

## Configuring

### Step 1: Set Redirect URI with your OIDC provider

Your OIDC provider will ask you for the following parameter:

- **Redirect URI**: Set to `https://coder.domain.com/api/v2/users/oidc/callback`

### Step 2: Configure Coder with the OpenID Connect credentials

See [configuring Coder](../configure.md) to learn how to modify your server configuration depending on your platform (e.g. Kubernetes, system service, etc).

Set the following environment variables for your Coder server:

```console
CODER_OIDC_ISSUER_URL="https://issuer.corp.com"
CODER_OIDC_EMAIL_DOMAIN="your-domain-1,your-domain-2"
CODER_OIDC_CLIENT_ID="533...des"
CODER_OIDC_CLIENT_SECRET="G0CSP...7qSM"
```

> Refer to our section below if you wish to authenticate [with a PKI](#pki-authentication-optional) instead of a client secret.

Restart your Coder server to apply this configuration. An OIDC authentication button should appear on your log in screen 🎉

![Log in with OIDC button](https://user-images.githubusercontent.com/22407953/261882891-7aa2e922-5572-490f-992a-07126bad0161.png)

## Provider Specific Notes

Any OIDC provider should work with Coder. With that being said, we have some special notes for specific providers.

- [Keycloak](./keycloak.md)
- [Active Directory Federation Services (ADFS)](./adfs.md)

## Group and Role Sync (enterprise)

Learn how to do [group and role sync](group-role-sync.md) with COder.

## How Coder Reads OIDC claims

When a user logs in for the first time via OIDC, Coder will merge both
the claims from the ID token and the claims obtained from hitting the
upstream provider's `userinfo` endpoint, and use the resulting data
as a basis for creating a new user or looking up an existing user.

To troubleshoot claims, set `CODER_VERBOSE=true` and follow the logs
while signing in via OIDC as a new user. Coder will log the claim fields
returned by the upstream identity provider in a message containing the
string `got oidc claims`, as well as the user info returned.

The following information is also avalible via a [sequence diagram](https://raw.githubusercontent.com/coder/coder/138ee55abb3635cb2f3d12661f8caef2ca9d0961/docs/images/oidc-sequence-diagram.svg) for visual learners.

> **Note:** If you need to ensure that Coder only uses information from
> the ID token and does not hit the UserInfo endpoint, you can set the
> configuration option `CODER_OIDC_IGNORE_USERINFO=true`.

### Email Addresses

By default, Coder will look for the OIDC claim named `email` and use that
value for the newly created user's email address.

If your upstream identity provider users a different claim, you can set
`CODER_OIDC_EMAIL_FIELD` to the desired claim.

> **Note:** If this field is not present, Coder will attempt to use the
> claim field configured for `username` as an email address. If this field
> is not a valid email address, OIDC logins will fail.

### Email Address Verification

Coder requires all OIDC email addresses to be verified by default. If
the `email_verified` claim is present in the token response from the identity
provider, Coder will validate that its value is `true`. If needed, you can
disable this behavior with the following setting:

```console
CODER_OIDC_IGNORE_EMAIL_VERIFIED=true
```

> **Note:** This will cause Coder to implicitly treat all OIDC emails as
> "verified", regardless of what the upstream identity provider says.

### Usernames

When a new user logs in via OIDC, Coder will by default use the value
of the claim field named `preferred_username` as the the username.

If your upstream identity provider uses a different claim, you can
set `CODER_OIDC_USERNAME_FIELD` to the desired claim.

> **Note:** If this claim is empty, the email address will be stripped of
> the domain, and become the username (e.g. `example@coder.com` becomes `example`).
> To avoid conflicts, Coder may also append a random word to the resulting
> username.

## OIDC Login Customization

If you'd like to change the OpenID Connect button text and/or icon, you can
configure them like so:

```console
CODER_OIDC_SIGN_IN_TEXT="Sign with Azure AD"
CODER_OIDC_ICON_URL="https://upload.wikimedia.org/wikipedia/commons/f/fa/Microsoft_Azure.svg"
```

![Custom OIDC text and icon](https://user-images.githubusercontent.com/22407953/261882846-1ce9c076-1247-4929-b082-72252b1f21c4.png)

### PKI Authentication (Optional)

An alternative authentication method is to use signed JWT tokens rather than a shared `client_secret`. This requires 2 files.

<blockquote class="warning">
  <p>
  Only <b>Azure AD</b> has been tested with this method. Other OIDC providers may not work, as most providers add additional requirements ontop of the standard that must be implemented. If you are using another provider and run into issues, please leave an issue on our <a href="https://github.com/coder/coder/issues">Github</a>.
  </p>
</blockquote>

- An RSA private key file
  - ```text
    -----BEGIN RSA PRIVATE KEY-----
    ... Base64 encoded key ...
    -----END RSA PRIVATE KEY-----
    ```
- The corresponding x509 certificate file
  - ```text
    -----BEGIN CERTIFICATE-----
    ... Base64 encoded x509 cert ...
    -----END CERTIFICATE-----
    ```

You must upload the public key (the certificate) to your OIDC provider.
Reference the documentation provided by your provider on how to do this. Depending on the provider, the name for this feature varies.

- <!-- Azure --> Authentication certificate credentials
- <!-- Okta --> JWT for Client Authentication
- <!-- Auth0 --> Authenticate with Private Key JWT

See [configuring Coder](../configure.md) to learn how to modify your server configuration depending on your platform (e.g. Kubernetes, system service, etc).

Set the following environment variables for your Coder server:

```console
CODER_OIDC_ISSUER_URL="https://issuer.corp.com"
CODER_OIDC_EMAIL_DOMAIN="your-domain-1,your-domain-2"
CODER_OIDC_CLIENT_KEY_FILE="/path/to/key.pem"
CODER_OIDC_CLIENT_CERT_FILE="/path/to/cert.pem"
```

Restart your Coder server to apply this configuration.

## Restrict OIDC Signups

If you wish to manually onboard users, or use a [script](../automation.md) to add users to Coder, set:

```console
CODER_OIDC_ALLOW_SIGNUPS=false
```

This will prevent new users from logging in via GitHub. An admin can manually add GitHub users from the "Users" page.
