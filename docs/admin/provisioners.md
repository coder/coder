# External provisioners

By default, the Coder server runs
[built-in provisioner daemons](../cli/server.md#provisioner-daemons), which
execute `terraform` during workspace and template builds. However, there are
sometimes benefits to running external provisioner daemons:

- **Secure build environments:** Run build jobs in isolated containers,
  preventing malicious templates from gaining shell access to the Coder host.

- **Isolate APIs:** Deploy provisioners in isolated environments (on-prem, AWS,
  Azure) instead of exposing APIs (Docker, Kubernetes, VMware) to the Coder
  server. See [Provider Authentication](../templates/authentication.md) for more
  details.

- **Isolate secrets**: Keep Coder unaware of cloud secrets, manage/rotate
  secrets on provisioner servers.

- **Reduce server load**: External provisioners reduce load and build queue
  times from the Coder server. See
  [Scaling Coder](./scale.md#concurrent-workspace-builds) for more details.

Each provisioner can run a single
[concurrent workspace build](./scale.md#concurrent-workspace-builds). For
example, running 30 provisioner containers will allow 30 users to start
workspaces at the same time.

Provisioners are started with the
[coder provisionerd start](../cli/provisionerd_start.md) command.

## Authentication

The provisioner daemon must authenticate with your Coder deployment.

Set a
[provisioner daemon pre-shared key (PSK)](../cli/server.md#--provisioner-daemon-psk)
on the Coder server and start the provisioner with
`coder provisionerd start --psk <your-psk>`. If you are
[installing with Helm](../install/kubernetes.md#install-coder-with-helm), see
the [Helm example](#example-running-an-external-provisioner-with-helm) below.

> Coder still supports authenticating the provisioner daemon with a
> [token](../cli.md#--token) from a user with the Template Admin or Owner role.
> This method is deprecated in favor of the PSK, which only has permission to
> access provisioner daemon APIs. We recommend migrating to the PSK as soon as
> practical.

## Types of provisioners

> Provisioners have two important tags: `scope` and `owner`. Coder sets these
> tags automatically.

### Organization-Scoped Provisioners

**Organization-scoped Provisioners** can pick up build jobs created by any user.
These provisioners always have tags `scope=organization owner=""`.

```shell
coder provisionerd start
```

### User-scoped Provisioners

**User-scoped Provisioners** can only pick up build jobs created from
user-tagged templates. User-scoped provisioners always have tags
`scope=owner owner=<uuid>`. Unlike the other provisioner types, any Coder user
can run user provisioners, but they have no impact unless there is at least one
template with the `scope=user` provisioner tag.

```shell
coder provisionerd start \
  --tag scope=user

# In another terminal, create/push
# a template that requires user provisioners
coder templates push on-prem \
  --provisioner-tag scope=user
```

### Provisioner Tags

You can use **provisioner tags** to control which provisioners can pick up build
jobs from templates (and corresponding workspaces) with matching tags.

```shell
coder provisionerd start \
  --tag environment=on_prem \
  --tag data_center=chicago

# In another terminal, create/push
# a template that requires this provisioner
coder templates push on-prem \
  --provisioner-tag environment=on_prem

# Or, match the provisioner exactly
coder templates push on-prem-chicago \
  --provisioner-tag environment=on_prem \
  --provisioner-tag data_center=chicago
```

A provisioner can run a given build job if one of the below is true:

1. The provisioner and job tags are both organization-scoped and both have no
   additional tags set,
1. The set of tags of the build job is a subset of the set of tags of the
   provisioner.

This is illustrated in the below table:

| Provisioner Tags                                                                      | Job Tags                                                                  | Can run job? |
| ------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- | ------------ |
| `{"owner":"","scope":"organization"}`                                                 | `{"owner":"","scope":"organization"}`                                     | true         |
| `{"owner":"","scope":"organization"}`                                                 | `{"environment":"on_prem","owner":"","scope":"organization"}`             | false        |
| `{"environment":"on_prem","owner":"","scope":"organization"}`                         | `{"owner":"","scope":"organization"}`                                     | false        |
| `{"environment":"on_prem","owner":"","scope":"organization"}`                         | `{"foo":"bar","owner":"","scope":"organization"}`                         | true         |
| `{"environment":"on_prem","owner":"","scope":"organization"}`                         | `{"data_center":"chicago","foo":"bar","owner":"","scope":"organization"}` | false        |
| `{"data_center":"chicago","environment":"on_prem","owner":"","scope":"organization"}` | `{"foo":"bar","owner":"","scope":"organization"}`                         | true         |
| `{"data_center":"chicago","environment":"on_prem","owner":"","scope":"organization"}` | `{"data_center":"chicago","foo":"bar","owner":"","scope":"organization"}` | true         |
| `{"owner":"aaa","scope":"owner"}`                                                     | `{"owner":"","scope":"organization"}`                                     | false        |
| `{"owner":"aaa","scope":"owner"}`                                                     | `{"owner":"aaa","scope":"owner"}`                                         | true         |
| `{"owner":"aaa","scope":"owner"}`                                                     | `{"owner":"bbb","scope":"owner"}`                                         | false        |
| `{"owner":"","scope":"organization"}`                                                 | `{"owner":"aaa","scope":"owner"}`                                         | false        |

The external provisioner in the above example can run build jobs with tags:

- `environment=on_prem`
- `data_center=chicago`
- `environment=on_prem datacenter=chicago`
- `environment=cloud datacenter=chicago`
- `environment=on_prem datacenter=new_york`

However, it will not pick up any build jobs that do not have either of the
`environment` or `datacenter` tags set. It will also not pick up any build jobs
from templates with the `user` tag set.

## Example: Running an external provisioner with Helm

Coder provides a Helm chart for running external provisioner daemons, which you
will use in concert with the Helm chart for deploying the Coder server.

1. Create a long, random pre-shared key (PSK) and store it in a Kubernetes
   secret

   ```shell
   kubectl create secret generic coder-provisioner-psk --from-literal=psk=`head /dev/urandom | base64 | tr -dc A-Za-z0-9 | head -c 26`
   ```

1. Modify your Coder `values.yaml` to include

   ```yaml
   provisionerDaemon:
     pskSecretName: "coder-provisioner-psk"
   ```

1. Redeploy Coder with the new `values.yaml` to roll out the PSK. You can omit
   `--version <your version>` to also upgrade Coder to the latest version.

   ```shell
   helm upgrade coder coder-v2/coder \
       --namespace coder \
       --version <your version> \
       --values values.yaml
   ```

1. Create a `provisioner-values.yaml` file for the provisioner daemons Helm
   chart. For example

   ```yaml
   coder:
     env:
       - name: CODER_URL
         value: "https://coder.example.com"
     replicaCount: 10
   provisionerDaemon:
     pskSecretName: "coder-provisioner-psk"
     tags:
       location: auh
       kind: k8s
   ```

   This example creates a deployment of 10 provisioner daemons (for 10
   concurrent builds) with the listed tags. For generic provisioners, remove the
   tags.

   > Refer to the
   > [values.yaml](https://github.com/coder/coder/blob/main/helm/provisioner/values.yaml)
   > file for the coder-provisioner chart for information on what values can be
   > specified.

1. Install the provisioner daemon chart

   ```shell
   helm install coder-provisioner coder-v2/coder-provisioner \
       --namespace coder \
       --version <your version> \
       --values provisioner-values.yaml
   ```

   You can verify that your provisioner daemons have successfully connected to
   Coderd by looking for a debug log message that says
   `provisionerd: successfully connected to coderd` from each Pod.

## Example: Running an external provisioner on a VM

```shell
curl -L https://coder.com/install.sh | sh
export CODER_URL=https://coder.example.com
export CODER_SESSION_TOKEN=your_token
coder provisionerd start
```

## Example: Running an external provisioner via Docker

```shell
docker run --rm -it \
  -e CODER_URL=https://coder.example.com/ \
  -e CODER_SESSION_TOKEN=your_token \
  --entrypoint /opt/coder \
  ghcr.io/coder/coder:latest \
  provisionerd start
```

## Disable built-in provisioners

As mentioned above, the Coder server will run built-in provisioners by default.
This can be disabled with a server-wide
[flag or environment variable](../cli/server.md#provisioner-daemons).

```shell
coder server --provisioner-daemons=0
```

## Prometheus metrics

Coder provisioner daemon exports metrics via the HTTP endpoint, which can be
enabled using either the environment variable `CODER_PROMETHEUS_ENABLE` or the
flag `--prometheus-enable`.

The Prometheus endpoint address is `http://localhost:2112/` by default. You can
use either the environment variable `CODER_PROMETHEUS_ADDRESS` or the flag
`--prometheus-address <network-interface>:<port>` to select a different listen
address.

If you have provisioners daemons deployed as pods, it is advised to monitor them
separately.
