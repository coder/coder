# Microsoft Entra ID authentication (OIDC)

This guide shows how to configure Coder to authenticate users with Microsoft Entra ID using OpenID Connect (OIDC)

## Prerequisites

- A Microsoft Azure Entra ID Tenant
- Permission to create Applications in your Azure environment

## Step 1: Create an OAuth App Registration in Microsoft Azure

1. Open Microsoft Azure Portal (https://portal.azure.com) → Microsoft Entra ID → App Registrations → New Registration
2. Name: Name your application appropriately
3. Supported Account Types: Choose the appropriate radio button according to your needs. Most organizations will want to use the first one labeled "Accounts in this organizational directory only"
4. Click on "Register"
5. On the next screen, select: "Certificates and Secrets"
6. Click on "New Client Secret" and under description, enter an appropriate description. Then set an expiry and hit "Add" once it's created, copy the value and save it somewhere secure for the next step
7. Next, click on the tab labeled "Token Configuration", then click "Add optional claim" and select the "ID" radio button, and finally check "upn" and hit "add" at the bottom
8. Then, click on the button labeled "Add groups claim" and check "Security groups" and click "Save" at the bottom
9. Now, click on the tab labeled "Authentication" and click on "Add a platform", select "Web" and for the redirect URI enter your Coder callback URL, and then hit "Configure" at the bottom:
   - `https://coder.example.com/api/v2/users/oidc/callback`

## Step 2: Configure Coder OIDC for Microsoft Entra ID

Set the following environment variables on your Coder deployment and restart Coder:

```env
CODER_OIDC_ISSUER_URL=https://login.microsoftonline.com/{tenant-id}/v2.0 # Replace {tenant-id} with your Azure tenant ID
CODER_OIDC_CLIENT_ID=<client id, located in "Overview">
CODER_OIDC_CLIENT_SECRET=<client secret, saved from step 6>
# Restrict to one or more email domains (comma-separated)
CODER_OIDC_EMAIL_DOMAIN="example.com"
CODER_OIDC_EMAIL_FIELD="upn" # This is set because EntraID typically uses .onmicrosoft.com domains by default, this should pull the user's username@domain email.
CODER_OIDC_GROUP_FIELD="groups" # This is for group sync / IdP Sync, a premium feature.
# Optional: customize the login button
CODER_OIDC_SIGN_IN_TEXT="Sign in with Microsoft Entra ID"
CODER_OIDC_ICON_URL=/icon/microsoft.svg
```

> [!NOTE]
> The redirect URI must exactly match what you configured in Microsoft Azure Entra ID

## Enable refresh tokens (recommended)

```env
# Keep standard scopes
CODER_OIDC_SCOPES=openid,profile,email,offline_access
```

After changing settings, users must log out and back in once to obtain refresh tokens

Learn more in [Configure OIDC refresh tokens](./refresh-tokens.md).

## Troubleshooting

- "invalid redirect_uri": ensure the redirect URI in Azure Entra ID matches `https://<your-coder-host>/api/v2/users/oidc/callback`
- Domain restriction: if users from unexpected domains can log in, verify `CODER_OIDC_EMAIL_DOMAIN`
- Claims: to inspect claims returned by Microsoft, see guidance in the [OIDC overview](./index.md#oidc-claims)

## See also

- [OIDC overview](./index.md)
- [Configure OIDC refresh tokens](./refresh-tokens.md)
