# Passwordless Authentication

You can create passwordless users in users for machine accounts. This can come in handy if you plan on [automating Coder](../automation.md) in CI/CD pipelines, for example.

In the Users page `https://coder.example.com/users`, create a new user with `Login Type: none`:

![Create new user](https://user-images.githubusercontent.com/22407953/262183871-9a9070fa-ca35-4816-9990-465b16b94fe4.png)

From there, you can create a long-lived token on behalf of the passwordless user using the [Create token API key](../../api/users.md#create-token-api-key):

```sh
# Replace API_KEY with a token from https://coder.example.com/cli-auth
curl -X POST http://coder-server:8080/api/v2/users/coder-bot/keys/tokens \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

Then, follow our documentation in [automating Coder](../automation.md) to perform actions on behalf of this user using their API token.
