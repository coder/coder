# Workloads in prebuilt workspaces

A proof of concept. `coder run --template <template> -- <command>` runs a
command, usually an AI agent, in a workspace of its own. The workspace is one
that the template keeps ready, so the command starts in a couple of seconds.
The workload's identity is an account of its own: it owns the workspace, and
it has nothing of the person who asked for it.

Everything is an ordinary Coder feature put to this use: a Terraform
template, prebuilt workspaces, a service account, workspace sharing and
scoped permissions. The parts that are new are listed under
[What is new](#what-is-new).

## How a run goes

1. A person, the sponsor, runs `coder run --template prebuilt -- <command>`.
1. coderd checks that the sponsor may make a workspace from the template.
1. coderd makes the identity: a service account that cannot sign in, has
   no token and is a plain member of the organization.
1. As that identity, coderd makes a workspace from the template. When the
   template keeps workspaces ready, one of them is claimed: its owner
   changes from `prebuilds` to the identity.
1. The identity shares the workspace with the sponsor, to use and not to
   own.
1. The daemon in the pod is told it has a new owner. It proves which pod it
   is with the kubelet's token, and coderd hands it the workload, with a
   token for the workload.
1. The command runs next to the daemon, with that token in its
   environment. The token is the identity's own. What it may do is what
   the sponsor asked for with `--scope`, and without that nothing.
1. When the command is over, the token is worth nothing, and coderd deletes
   the workspace and retires the identity. The record of what ran stays.

## Files

| File                                             | Purpose                                                   |
|--------------------------------------------------|-----------------------------------------------------------|
| [template/main.tf](template/main.tf)             | The template: a pod, pod identity, and how many are ready |
| [cluster/namespace.yaml](cluster/namespace.yaml) | The namespace of the pods                                 |
| [cluster/coder.yaml](cluster/coder.yaml)         | What lets coderd ask the cluster whose token a pod has    |
| [cluster/users.yaml](cluster/users.yaml)         | A name and a shell for the user the pods run as           |
| [validate.py](validate.py)                       | Checks a running setup, from inside and outside           |
| [RESULTS.md](RESULTS.md)                         | What the checks showed, and the limits                    |

## Set up

coderd needs a Premium license, for prebuilt workspaces and service
accounts, and the `runtime-workloads` experiment. It also needs to ask the
cluster whose token a pod presents. Inside the cluster, run coderd as the
service account `coder-server` and set only the namespace. Outside, give it
the API server's URL, its certificate authority and a token of that
service account.

```sh
kubectl apply -f cluster/namespace.yaml -f cluster/coder.yaml -f cluster/users.yaml
kubectl -n coder-sandbox-workloads create token coder-server --duration 720h > kube-token

CODER_EXPERIMENTS=runtime-workloads \
CODER_POD_IDENTITY_NAMESPACE=coder-sandbox-workloads \
CODER_POD_IDENTITY_KUBE_URL=https://<api-server> \
CODER_POD_IDENTITY_KUBE_CA_FILE=ca.crt \
CODER_POD_IDENTITY_KUBE_TOKEN_FILE=kube-token \
coder server

coder templates push prebuilt -d template \
  --variable coder_url=https://coder.example.com --yes
```

## Try it

```sh

# Who am I in there?
coder run --template prebuilt --preset agent -- coder identity

# Let it read templates, and nothing else.
coder run --template prebuilt --preset agent --scope template:read -- coder templates list

# Keep the workspace, and go and look.
coder run --template prebuilt --preset agent --keep --detach --name fix-1 -- sleep 3600
coder run --all
coder ssh workload-<id>/fix-1
coder run --remove fix-1
```

`CODER_WORKSPACE_PREBUILDS_RECONCILIATION_INTERVAL` decides how soon a
claimed workspace is replaced. The default is one minute.

## What is new

- **`POST /api/v2/workloads`.** One request makes the identity, the
  workspace and the workload. The sponsor's command line never holds
  anything of the workload's.
- **An identity can own a workspace.** It is a service account, recorded
  with its sponsor and scopes.
- **The workload has a token of its own.** It is an API token of the
  identity's account, handed to the daemon that proved which pod it is, and
  to nobody else. What it may do is read from the identity's record
  whenever it is used. It is refused once the workload is over.
- **The sponsor's access to the template counts.** coderd makes and deletes
  the workspace as the identity, and lets it use the template because its
  sponsor may. The template need not be open to everybody.
- **Pod identity.** The daemon proves which pod it is with the kubelet's
  token, and coderd asks the cluster whose token it is. Pods outside the
  namespace coderd was given are refused. The template holds no token. A prebuilt workspace's daemon gets a new credential when the
  workspace is claimed, because the pod is found by its UID and not by a
  token.
- **No session token for an identity.** A build normally hands Terraform a
  token for the workspace's owner. For an identity it does not, since such
  a token would ignore the workload's scopes.
- **The sponsor can end what they sponsor**, though they do not own the
  workspace. Removing the workload deletes its workspace, also one that
  was kept.
- **An identity is retired with its workspace**, whoever deletes it.

## Limits

- **A workload cannot read a template that is not open to its identity.**
  Reading a workspace takes reading its template, so in a template that is
  restricted to groups a workload cannot read its own workspace either,
  whatever its scopes.
- **The template must not need the owner's external authentication.** An
  identity has none, by design.
- **coderd believes the template about which pod is which agent's.** A
  template registers a pod's UID as its agent's instance. Whoever may
  write templates could register somebody else's pod.
- **The template names another kind of instance identity.** The Terraform
  provider has no `kubernetes` authentication type yet.
- **The workload and its pod's daemon are one principal.** The workload can
  read the pod's token, and with it be the daemon.
- **A sponsor who enters the workspace is inside the workload's pod** and
  can act as the workload.
- **A workload cannot start workloads.** What a child may inherit is not
  designed.
- **A workspace that is kept keeps its identity** until the workload is
  removed or the workspace is deleted.
- **The pods' network is not restricted** by this template.
- **The license is not checked** by the new endpoint.
