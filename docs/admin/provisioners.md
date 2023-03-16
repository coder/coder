# External provisioners

By default, the Coder server runs [built-in provisioner daemons](../cli/coder_server.md#provisioner-daemons), which execute `terraform` during workspace and template builds. However, there are sometimes benefits to running external provisioner daemons:

- **Secure build environments:** Run build jobs in isolated containers, preventing malicious templates from gaining shell access to the Coder host.

- **Isolate APIs:** Deploy provisioners in isolated environments (on-prem, AWS, Azure) instead of exposing APIs (Docker, Kubernetes, VMware) to the Coder server. See [Provider Authentication](../templates/authentication.md) for more details.

- **Isolate secrets**: Keep Coder unaware of cloud secrets, manage/rotate secrets on provisoner servers.

- **Reduce server load**: External provisioners reduce load and build queue times from the Coder server. See [Scaling Coder](./scale.md#concurrent-workspace-builds) for more details.

> External provisioners are in an [alpha state](../contributing/feature-stages.md#alpha-features) and the behavior is subject to change. Use [GitHub issues](https://github.com/coder/coder) to leave feedback.

## Running external provisioners

Each provisioner can run a single [concurrent workspace build](./scale.md#concurrent-workspace-builds). For example, running 30 provisioner containers will allow 30 users to start workspaces at the same time.

### Requirements

- The [Coder CLI](../cli.md) must installed on and authenticated as a user with the Owner or Template Admin role.
- Your environment must be [authenticated](../templates/authentication.md) against the cloud environments templates need to provision against.

### Types of provisioners

- **Generic provisioners** can pick up any build job from templates without provisioner tags.

  ```sh
  coder provisionerd start
  ```

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

### Example: Running an external provisioner on a VM

```sh
curl -L https://coder.com/install.sh | sh
export CODER_URL=https://coder.example.com
export CODER_SESSION_TOKEN=your_token
coder provisionerd start
```

### Example: Running an external provisioner via Docker

```sh
docker run --rm -it \
  -e CODER_URL=https://coder.example.com/ \
  -e CODER_SESSION_TOKEN=your_token \
  --entrypoint /opt/coder \
  ghcr.io/coder/coder:latest \
  provisionerd start
```

## Disable built-in provisioners

As mentioned above, the Coder server will run built-in provisioners by default. This can be disabled with a server-wide [flag or environment variable](../cli/coder_server.md#provisioner-daemons).

```sh
coder server --provisioner-daemons=0
```
