# How to use

Start the idp service:

```bash
$ go run main.go
2024-01-10 16:48:01.415 [info]  stdlib: 2024/01/10 10:48:01 IDP Issuer URL http://127.0.0.1:44517
2024-01-10 16:48:01.415 [info]  stdlib: 2024/01/10 10:48:01 Oauth Flags
2024-01-10 16:48:01.415 [info]  stdlib: 2024/01/10 10:48:01 --external-auth-providers='[{"type":"fake","client_id":"f2df566b-a1c9-407a-8b75-480db45c6476","client_secret":"55aca4e3-7b94-44b6-9f45-ecb5e81c560d","auth_url":"http://127.0.0.1:44517/oauth2/authorize","token_url":"http://127.0.0.1:44517/oauth2/token","validate_url":"http://127.0.0.1:44517/oauth2/userinfo","scopes":["openid","email","profile"]}]'
2024-01-10 16:48:01.415 [info]  stdlib: 2024/01/10 10:48:01 Press Ctrl+C to exit
```

Then use the flag into your coderd instance:

```bash
develop.sh -- --external-auth-providers='[{"type":"fake","client_id":"f2df566b-a1c9-407a-8b75-480db45c6476","client_secret":"55aca4e3-7b94-44b6-9f45-ecb5e81c560d","auth_url":"http://127.0.0.1:44517/oauth2/authorize","token_url":"http://127.0.0.1:44517/oauth2/token","validate_url":"http://127.0.0.1:44517/oauth2/userinfo","scopes":["openid","email","profile"]}]'
```
