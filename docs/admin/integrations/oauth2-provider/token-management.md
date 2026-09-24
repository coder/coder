---
title: OAuth2 provider token management
---

Refresh an access token, revoke a token, revoke your own authorization for an application, and delete an application.
For the authentication methods used in these requests, refer to [Client Authentication Methods](./integration-patterns.md#client-authentication-methods).

## Refresh Tokens

Refresh an expired access token.

**Option A: HTTP Basic authentication (`client_secret_basic`)**

```sh
curl -X POST \
  -u "$CLIENT_ID:$CLIENT_SECRET" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=refresh_token" \
  -d "refresh_token=$REFRESH_TOKEN" \
  "$CODER_URL/oauth2/tokens"
```

**Option B: Form parameters (`client_secret_post`)**

```sh
curl -X POST \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=refresh_token" \
  -d "refresh_token=$REFRESH_TOKEN" \
  -d "client_id=$CLIENT_ID" \
  -d "client_secret=$CLIENT_SECRET" \
  "$CODER_URL/oauth2/tokens"
```

**Option C: Public client (`none`)**

```sh
curl -X POST \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=refresh_token" \
  -d "refresh_token=$REFRESH_TOKEN" \
  -d "client_id=$CLIENT_ID" \
  "$CODER_URL/oauth2/tokens"
```

## Revoke a Token

Revoke one refresh token or access token through the
[RFC 7009](https://datatracker.ietf.org/doc/html/rfc7009) endpoint that
`revocation_endpoint` advertises. A confidential client authenticates as it
does on a refresh, with HTTP Basic as below or with `client_id` and
`client_secret` form fields as in the refresh examples above. An omitted or
wrong secret answers HTTP 401 with `error=invalid_client`:

```sh
curl -X POST \
  -u "$CLIENT_ID:$CLIENT_SECRET" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=$REFRESH_TOKEN" \
  "$CODER_URL/oauth2/revoke"
```

A public client sends `client_id` alone. Revoking a refresh token also ends the
access token issued with it. A successful revocation returns HTTP 200, but that
response does not confirm that the token existed or belonged to your client. A
confidential client that fails to authenticate receives HTTP 401 with
`error=invalid_client` and nothing is revoked.

## Revoke your authorization for an application

Revoke your own authorization codes and tokens for an application, using the session token that authorized them:

```sh
curl -X DELETE \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  "$CODER_URL/oauth2/tokens?client_id=$CLIENT_ID"
```

This ends your own sessions with the application but leaves the application registered, so it can authorize again, and does not affect any other user's tokens for it.
For a cutoff that affects every user, delete the application or one of its client secrets, as described under [Delete an Application](#delete-an-application).

## Delete an Application

Deleting an application is a separate operation from revoking a single user's tokens.
It removes the registration itself, so the client cannot authorize again without being registered anew, and it revokes every token issued under it, for every user.
Deleting one of an application's client secrets has the same effect on every token issued under that secret.

In the web UI, navigate to **Deployment Settings** > **OAuth2 Applications**, select the application on the **Applications** tab, then select **Delete**.
This requires permission to delete OAuth2 applications.

Or with the management API:

```sh
curl -X DELETE \
  -H "Authorization: Bearer $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/v2/oauth2-provider/apps/$APP_ID"
```

This is also how you remove clients that registered themselves while dynamic client registration was enabled.
Turning the setting off stops new registrations; it does not remove the ones already there.

## Learn more

- Check [Common issues](./troubleshooting.md) if a refresh or revocation fails
