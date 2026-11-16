# Delegated identities

A proof of concept. It is not production code.

A delegated identity is an identity created under another identity, its
parent, that can never do more than its parent. It lets something that acts
for a person, such as a coding agent, be itself to Coder instead of being the
person. The identity owns its workspaces and chats, is named in audit logs,
and holds only what it was given.

Nothing about the agent's compute is special. An agent runs in an ordinary
workspace of an ordinary template. Whoever owns the workspace is who the
agent in it is to Coder.

## The model

- **A delegated identity is an account of its own**, a service account with
  no way to sign in, plus a record of its parent, its root user and its
  scopes.
- **Its parent is a user or another delegated identity.** The chain ends at
  a user, the root user.
- **Creating one returns a record, not a credential.** The identity gets its
  credentials where it is used.
- **It is judged with three things**, read on every request:
  - its own ID, so it owns what it creates and cannot reach what its root
    user owns;
  - the roles and groups of its root user, so it can never reach further
    than that user, and a template open to a group stays open to it;
  - its own scopes, less whatever a delegated identity above it lacks, so
    lowering a parent lowers everything below it at once.
- **An identity without scopes can do nothing with the API.** Its workspace
  daemon still works: what a daemon needs does not come out of the scopes.
- **Acting on behalf of an identity** takes two things: the `use` permission
  on delegated identities, and being above the identity. Someone who is not
  above it needs the permission across the organization, which is how a
  trusted service acts for members. The caller is recorded as who asked,
  and the identity as the owner.

## Where it is used

| Where       | How an agent comes to act under a delegated identity                                                                                   |
|-------------|----------------------------------------------------------------------------------------------------------------------------------------|
| Workspaces  | `coder create <identity>/<name>` creates a workspace on behalf of the identity. The session token in it is the identity's.             |
| Chats       | A chat is created with the identity as its owner. Its tools create workspaces the identity owns, and its model calls carry its key.    |
| Agent Relay | The relay's service account creates an identity under the developer it matched, and builds the session's workspace on behalf of it.    |

In all three the accounts above the identity can enter its workspaces,
through workspace sharing, and the identity gets nothing of theirs in
return.

## What a template sees

A template does not need to know about delegated identities. One that wants
to tell can read four attributes of `data.coder_workspace_owner`:

| Attribute        | The owner is a person | The owner is a delegated identity                   |
|------------------|-----------------------|-----------------------------------------------------|
| `delegated`      | `false`               | `true`                                              |
| `parent_id`      | Empty                 | The ID of the identity it was created under         |
| `root_user_id`   | The owner's ID        | The ID of its root user                             |
| `root_user_name` | The owner's username  | The username of its root user                       |

The parent is the root user, or another delegated identity in between.

```hcl
data "coder_workspace_owner" "me" {}

# Label what an agent owns with the person who answers for it.
locals {
  labels = {
    "coder.owner"     = data.coder_workspace_owner.me.name
    "coder.root-user" = data.coder_workspace_owner.me.root_user_name
    "coder.delegated" = tostring(data.coder_workspace_owner.me.delegated)
  }
}
```

The attributes need the branch `feat/workspace-owner-delegated-identity` of
the Coder Terraform provider. With a released provider the template still
works, and cannot tell.

## Try it

From a checkout of this branch, start a development server with the
experiment on. It creates an `admin` user and a `docker` template, and
prints the alias to reach it with.

```sh
./scripts/develop.sh -- --experiments delegated-identities
alias cdr=./scripts/coder-dev.sh
```

```sh
# A delegated identity under you. It may do nothing yet.
cdr identities create my-agent

# A workspace on its behalf. You asked, and it owns the workspace.
cdr create my-agent/task-1 --template docker --use-parameter-defaults -y

# You can enter it: it is shared with the accounts above its owner.
cdr ssh my-agent/task-1

# Act as the identity. An administrator can make a token for it to try
# this out. Without scopes it sees nothing.
TOKEN=$(cdr tokens create --user my-agent --name demo 2>/dev/null | tail -1)
cdr --token "$TOKEN" identities show me
cdr --token "$TOKEN" list

# Let it read its workspaces. This applies to the token it already has.
cdr identities update my-agent --scope workspace:read --scope template:read
cdr --token "$TOKEN" list

# What it was not given stays refused.
cdr --token "$TOKEN" delete my-agent/task-1 -y

# Clean up. An identity that owns something cannot be deleted.
cdr delete my-agent/task-1 -y
cdr identities delete my-agent
```

The `docker` template gives the workspace no credential of its owner's, which
is why the token above is made by hand. The template in this directory hands
the owner's session token to the workspace, so the command line inside it is
the identity:

```sh
coder create my-agent/task-1 --template prebuilt --preset agent -y
coder ssh my-agent/task-1 -- coder identities show me
```

A chat under the identity needs a model to be configured. The Go test
`TestDelegatedIdentitiesChat` runs one end to end. This request was not run
by hand:

```sh
coder exp api POST /api/v2/chats organization_id=<org> owner_id=<identity id> \
  'content:=[{"type":"text","text":"Create a workspace."}]'
```

## What is in the branch

| Part                         | Where                                                                                      |
|------------------------------|--------------------------------------------------------------------------------------------|
| Schema and queries           | `coderd/database/migrations/000642_delegated_identities.up.sql`, `queries/delegatedidentities.sql` |
| How an identity is judged    | `coderd/httpmw/delegatedidentity.go`, `subjectWithDelegation` in `coderd/httpmw/apikey.go`       |
| API and the on-behalf rule   | `coderd/delegatedidentities.go` (`/api/v2/identities`)                                     |
| Workspaces                   | `postUserWorkspaces` in `coderd/workspaces.go`, `postWorkspaceBuilds` in `coderd/workspacebuilds.go` |
| Chats                        | `createChat` and the workspace hooks in `coderd/exp_chats.go`, `coderd/chat_routes.go`     |
| Permission                   | The `delegated_identity` resource in `coderd/rbac/policy/policy.go`                        |
| Spend and quota              | `spendOwner` in `coderd/aibridgedserver`, `enterprise/coderd/workspacequota.go`            |
| Cleanup                      | `coderd/database/dbpurge`                                                                  |
| Command line                 | `cli/identities.go` (`coder identities`)                                                   |
| Tests                        | `coderd/delegatedidentities_test.go`, `coderd/httpmw/delegatedidentity_test.go`            |
| A check of a live deployment | [validate.py](validate.py), with what it showed in [RESULTS.md](RESULTS.md)                |

Agent Relay is a separate program in a separate repository and is not
changed here. A test acts as a service account with the permissions the
relay's documentation lists, plus the four new ones, and does what the relay
would do.

## Set up the example template

Delegated identities need the `delegated-identities` experiment and nothing
else. Any template works. The template here also shows two things that are
independent of them: workspaces that are built ahead of time, and a daemon
that proves which pod it is. For those, coderd needs a Premium license. It needs no way to the
cluster's API and no credential for the cluster. It is given a key, and an
operator in the cluster registers the cluster with that key.

```sh
head -c 32 /dev/urandom | base64 > operator-key

CODER_EXPERIMENTS=delegated-identities \
CODER_POD_IDENTITY_OPERATOR_KEY="$(cat operator-key)" \
coder server

kubectl apply -f cluster/namespace.yaml -f cluster/pods.yaml -f cluster/users.yaml
kubectl -n coder-sandbox-workloads create secret generic \
  coder-pod-identity-operator --from-file=key=operator-key
CODER_URL=https://coder.example.com CLUSTER_NAME=my-cluster \
  envsubst < cluster/operator.yaml | kubectl apply -f -

coder templates push prebuilt -d template --yes
```

The template says `auth = "kubernetes-pod-identity"`, which the Coder
Terraform provider does not accept yet.
[provider/kubernetes-pod-identity.patch](provider/kubernetes-pod-identity.patch)
adds it to version 2.20.0 of the provider. Build the provider with the
patch and point Terraform at it with a filesystem mirror, in the file
`TF_CLI_CONFIG_FILE` names for coderd:

```hcl
provider_installation {
  filesystem_mirror {
    path    = "/path/to/mirror"
    include = ["registry.terraform.io/coder/coder"]
  }
  direct {
    exclude = ["registry.terraform.io/coder/coder"]
  }
}
```

The provider binary goes to
`<mirror>/registry.terraform.io/coder/coder/2.20.1/linux_amd64/terraform-provider-coder_v2.20.1`.

## How coderd believes a pod (not part of delegated identities)

1. **The operator registers the cluster.** It runs in the cluster and reads
   what every service account there may read: the issuer of the cluster's
   service account tokens and the public keys they are signed with. It
   sends these to coderd with the operator key, every 30 seconds.
1. **The kubelet gets the pod a token.** The pod's spec asks for a token
   for coderd's URL. The cluster signs it and binds it to the pod.
1. **The daemon presents the token to coderd.**
1. **coderd checks it against the registered keys.** The signature, the
   audience, the expiry, that it is bound to a pod, and that the pod is in
   a namespace the operator named. coderd asks the cluster nothing.
1. **The pod's UID finds the agent** that the template registered it for.

coderd stops believing a cluster two minutes after its operator last
renewed the registration.
