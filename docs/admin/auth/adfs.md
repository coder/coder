# Active Directory Federation Services OIDC with Coder (ADFS)

> **Note:** Tested on ADFS 4.0, Windows Server 2019

1. In your Federation Server, create a new application group for Coder. Follow the
   steps as described [here.](https://learn.microsoft.com/en-us/windows-server/identity/ad-fs/development/msal/adfs-msal-web-app-web-api#app-registration-in-ad-fs)
   - **Server Application**: Note the Client ID.
   - **Configure Application Credentials**: Note the Client Secret.
   - **Configure Web API**: Set the Client ID as the relying party identifier.
   - **Application Permissions**: Allow access to the claims `openid`, `email`, `profile`, and `allatclaims`.
1. Visit your ADFS server's `/.well-known/openid-configuration` URL and note
   the value for `issuer`.
   > **Note:** This is usually of the form `https://adfs.corp/adfs/.well-known/openid-configuration`
1. In Coder's configuration file (or Helm values as appropriate), set the following
   environment variables or their corresponding CLI arguments:

   - `CODER_OIDC_ISSUER_URL`: the `issuer` value from the previous step.
   - `CODER_OIDC_CLIENT_ID`: the Client ID from step 1.
   - `CODER_OIDC_CLIENT_SECRET`: the Client Secret from step 1.
   - `CODER_OIDC_AUTH_URL_PARAMS`: set to

     ```console
     {"resource":"$CLIENT_ID"}
     ```

     where `$CLIENT_ID` is the Client ID from step 1 ([see here](https://learn.microsoft.com/en-us/windows-server/identity/ad-fs/overview/ad-fs-openid-connect-oauth-flows-scenarios#:~:text=scope%E2%80%AFopenid.-,resource,-optional)).
     This is required for the upstream OIDC provider to return the requested claims.

   - `CODER_OIDC_IGNORE_USERINFO`: Set to `true`.

1. Configure [Issuance Transform Rules](https://learn.microsoft.com/en-us/windows-server/identity/ad-fs/operations/create-a-rule-to-send-ldap-attributes-as-claims)
   on your federation server to send the following claims:

   - `preferred_username`: You can use e.g. "Display Name" as required.
   - `email`: You can use e.g. the LDAP attribute "E-Mail-Addresses" as required.
   - `email_verified`: Create a custom claim rule:

     ```console
     => issue(Type = "email_verified", Value = "true")
     ```

   - (Optional) If using Group Sync, send the required groups in the configured groups claim field. See [here](https://stackoverflow.com/a/55570286) for an example.
