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
   is with the kubelet's token, and coderd hands it the workload.
1. The command runs next to the daemon. Its requests to Coder are made as
   the identity, narrowed to the scopes the sponsor asked for. Without
   `--scope` it can do nothing.
1. When the command is over, coderd deletes the workspace and retires the
   identity. The record of what ran stays.

## Files

| File                                     | Purpose                                                   |
|------------------------------------------|-----------------------------------------------------------|
| [template/main.tf](template/main.tf)     | The template: a pod, pod identity, and how many are ready |
| [cluster/users.yaml](cluster/users.yaml) | A name and a shell for the user the pods run as           |
| [validate.py](validate.py)               | Checks a running setup, from inside and outside           |
| [RESULTS.md](RESULTS.md)                 | What the checks showed, and the limits                    |

The cluster side is the namespace and the pods' service account from
[../sandbox-workloads/cluster](../sandbox-workloads/cluster), and coderd's
access to the cluster's TokenReview as described in
[../sandbox-workloads/README.md](../sandbox-workloads/README.md). The Agent
Sandbox controller and its warm pool are not needed.

## Try it

coderd needs a Premium license, for prebuilt workspaces and service
accounts, and the `runtime-workloads` experiment.

```sh
kubectl apply -f cluster/users.yaml
coder templates push prebuilt -d template \
  --variable coder_url=https://coder.example.com --yes

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
- **Pod identity.** The daemon proves which pod it is. The template holds no
  token. A prebuilt workspace's daemon gets a new credential when the
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

- **The template must be usable by every member of the organization.** The
  identity makes the workspace as itself. A template that is restricted to
  groups needs a rule that lets the sponsor's access count.
- **The template must not need the owner's external authentication.** An
  identity has none, by design.
- **The workload and its pod's daemon are one principal.** The workload can
  read the pod's token.
- **A sponsor who enters the workspace is inside the workload's pod** and
  can act as the workload.
- **A workload cannot start workloads.** What a child may inherit is not
  designed.
- **A workspace that is kept keeps its identity** until the workload is
  removed or the workspace is deleted.
- **The pods' network is not restricted** by this template.
- **The license is not checked** by the new endpoint.
