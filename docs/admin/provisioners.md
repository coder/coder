# External provisioners

By default, the Coder server runs [built-in provisioner daemons](../cli/coder_server.md#provisioner-daemons), which execute `terraform` during workspace and template builds. You can learn more about `provisionerd` in our [architecture documentation](../about/architecture.md#provisionerd).

> While external provisioners are stable, the feature is in an [alpha state](../contributing/feature-stages.md#alpha-features) and the behavior is subject to change in future releases. Use [GitHub issues](https://github.com/coder/coder) to leave feedback.

## Benefits of external provisioners

There are benefits in running external provisioner servers.

### Security

As you add more (template) admins in Coder, there is an increased risk of malicious code being added into templates. Isolated provisioners can prevent template admins from running code directly against the Coder server, database, or host machine.

Additionally, you can configure provisioner environments to access cloud secrets that you would like to conceal from the Coder server.

### Extensibility

Instead of exposing an entire API and secrets (e.g. Kubernetes, Docker, VMware) to the Coder server, you can run provisioners in each environment. See [Provider authentication](../templates/authentication.md) for more details.

### Scalability

External provisioners can reduce load and build queue times from the Coder server. See [Scaling Coder](./scale.md#concurrent-workspace-builds) for more details.

## Run an external provisioner

Once authenticated as a user with the Template Admin or Owner role, the [Coder CLI](../cli.md) can launch external provisioners. There are 3 types of provisioners:

- **Generic provisioners** can pick up any build job from templates without provisioner tags.

  ```sh
  coder provisionerd start
  ```

  > Ensure all provisioners (including [built-in provisioners](#disable-built-in-provisioners)) have similar configuration/cloud access. Otherwise, users may run into intermittent build errors depending on which provisioner picks up a job.

- **Tagged provisioners** can be used to pick up build jobs from templates (and corresponding workspaces) with matching tags.

  ```sh
  coder provisionerd start \
    --tag environment=on_prem \
    --tag data_center=chicago

  # In another terminal, create/push
  # a template that requires this provisioner
  coder templates create on-prem \
    --provisioner-tag environment=on_prem

  # Or, match the provisioner exactly
  coder templates create on-prem-chicago \
    --provisioner-tag environment=on_prem \
    --provisioner-tag data_center=chicago
  ```

  > At this time, tagged provisioners can also pick jobs from untagged templates. This behavior is [subject to change](https://github.com/coder/coder/issues/6442).

- **User provisioners** can only pick up jobs from user-tagged templates. Unlike the other provisioner types, any Coder can run user provisioners, but they have no impact unless there is at least one template with the `scope=user` provisioner tag.

  ```sh
  coder provisionerd start \
    --tag scope=user

  # In another terminal, create/push
  # a template that requires user provisioners
  coder templates create on-prem \
    --provisioner-tag scope=user
  ```

## Running external provisioners via Docker

The following command can run a Coder provisioner isolated in a Docker container.

```sh
docker run --rm -it \
  -e CODER_URL=https://coder.example.com/ \
  -e CODER_SESSION_TOKEN=your_token \
  --entrypoint /opt/coder \
  ghcr.io/coder/coder:latest \
  provisionerd start
```

Be sure to replace `https://coder.example.com` with your [access URL](./configure.md#access-url) and `your_token` with an [API token](../api.md).

To include [provider secrets](../templates/authentication.md), modify the `docker run` command to mount environment variables or external volumes. Alternatively, you can create a custom provisioner image.

## Disable built-in provisioners

As mentioned above, the Coder server will run built-in provisioners by default. This can be disabled with a server-wide [flag or environment variable](../cli/coder_server.md#provisioner-daemons).

```sh
coder server --provisioner-daemons=0
```
