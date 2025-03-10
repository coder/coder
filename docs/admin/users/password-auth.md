# Password Authentication

Coder has password authentication enabled by default. The account created during
setup is a username/password account.

## Disable password authentication

To disable password authentication, use the
[`CODER_DISABLE_PASSWORD_AUTH`](../../reference/cli/server.md#--disable-password-auth)
flag on the Coder server.

## Restore the `Owner` user

If you remove the admin user account (or forget the password), you can run the
[`coder server create-admin-user`](../../reference/cli/server_create-admin-user.md)command
on your server.

> [!NOTE]
> You must run this command on the same machine running the Coder server.
> If you are running Coder on Kubernetes, this means using
> [kubectl exec](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_exec/)
> to exec into the pod.

## Reset a user's password

An admin must reset passwords on behalf of users. This can be done in the web UI
in the Users page or CLI:
[`coder reset-password`](../../reference/cli/reset-password.md)
