# A workspace of its own for every workload

A spike, behind the `runtime-workloads` experiment. It builds on
[workloads inside a workspace daemon](../runtime-workloads/README.md) and
changes where a workload runs.

`coder run --template sandbox -- <command>` gives the command a **workspace
of its own**: a pod claimed from a warm pool that the Kubernetes
[Agent Sandbox](https://agent-sandbox.sigs.k8s.io) project keeps ready. The
workspace is made for this one workload and deleted when the command is
over. There is no workspace to run the workload "in" and nothing to nest:
the workload is the workspace.

- **It is fast.** No Terraform runs and no pod is scheduled. From the
  command line to the exit code takes under a second.
- **The cluster says who the workload is.** The pod's daemon holds no token
  from Coder. It presents the token the kubelet obtained for its pod, and
  coderd asks the cluster which pod that is.
- **The pod is the sandbox.** It meets the Restricted Pod Security
  Standard, has resource limits of its own, and can reach Coder and nothing
  else.

Everything else is as before: the workload has an identity sponsored by the
user who ran it, can do nothing in Coder without scopes, and leaves a record
and audit entries.

| File                                                     | Purpose                                                  |
|----------------------------------------------------------|----------------------------------------------------------|
| [template/sandbox.yaml](template/sandbox.yaml)           | The Coder template: it names a warm pool                 |
| [cluster/namespace.yaml](cluster/namespace.yaml)         | The namespace, with the Restricted standard enforced     |
| [cluster/controller.yaml](cluster/controller.yaml)       | The Agent Sandbox controller, limited to the namespace   |
| [cluster/coder.yaml](cluster/coder.yaml)                 | What coderd may do in the cluster, and the pods' account |
| [cluster/pool.yaml](cluster/pool.yaml)                   | The warm pool: what a workload's pod is                  |
| [template-terraform/main.tf](template-terraform/main.tf) | The same pool, claimed by an ordinary Terraform template |
| [validate.py](validate.py)                               | Checks a running setup, from inside and outside          |
| [RESULTS.md](RESULTS.md)                                 | What the checks showed, the requirements, and the limits |
| [DESIGN.md](DESIGN.md)                                   | Ways to define the pool in Terraform instead             |

## How it works

```text
          the platform team                      Agent Sandbox controller
kubectl apply pool.yaml ───────────────────────► keeps N pods running. In each, a
                                                 Coder daemon that is nobody yet asks
                                                 coderd: "this is my pod's token, am I
                                                 anybody?" and is told to wait.

coder run --template sandbox --scope <scope> -- <command>
   │
   ▼
coderd  creates a workspace from the template. Its build is done by the
   │    sandbox provisioner, which runs no Terraform:
   │      1. claims a sandbox from the pool         (a SandboxClaim)
   │      2. follows the claim to its pod           (claim → sandbox → pod)
   │      3. registers the pod's UID as the workspace daemon's instance ID
   ▼
the waiting daemon is answered: coderd has the cluster review its token, finds
   │    the pod's UID on the new workspace, and hands over the daemon's session
   ▼
coderd  creates the workload identity (sponsor, scopes) for that daemon
   │
   ▼
the daemon fetches the workload, runs the command next to itself, and reports
        the outcome. coderd records it and deletes the workspace, which gives
        the pod up. The pool starts a new one.
```

- **A sandbox template** is one file, [`sandbox.yaml`](template/sandbox.yaml),
  that names a warm pool. `coder templates push` recognizes it. Workspaces of
  it are built by the `sandbox` provisioner
  ([`provisioner/sandbox`](../../provisioner/sandbox/serve.go)) instead of
  Terraform. They are ordinary workspaces otherwise: they are listed, can be
  entered with `coder ssh`, stopped, started and deleted.
- **The pool** decides what a workload's pod is: image, resource limits,
  network policy, security context and, where the cluster has one, a
  sandbox runtime such as gVisor or Kata. A template author chooses a pool.
  A user chooses a template.
- **Pod identity** is a new way for a workspace daemon to authenticate,
  `CODER_AGENT_AUTH=kubernetes`, next to the instance identities of the
  cloud providers. The daemon sends its pod's projected service account
  token. coderd sends it to the cluster's TokenReview API and accepts it
  only if it was issued for Coder and is bound to a pod. The pod's UID must
  be the instance ID of a daemon in the latest build of a workspace. A pod
  that nobody has claimed is told to wait, and is answered the moment a
  build registers it.
- **The daemon fetches its workloads.** coderd does not call the daemon to
  start a workload any more. A daemon that has just been claimed takes a
  moment to become reachable, and the workload does not wait for that.
- **The pod is the sandbox.** The daemon runs the command as a process
  next to itself (the `pod` runtime). The workload and its daemon are one
  principal: what the cluster vouches for is the pod.
- **The record** is the same as before, and `coder run --list`, `--all`,
  `--logs` and `--remove` work across workspaces when no workspace is
  named. The workspace of a finished workload is gone; its record is not.

`coder run <workspace> -- <command>` still runs a workload in a sandbox
inside an existing workspace's daemon. The two are tiers of one thing.

The sandbox provisioner is the shortcut of this spike: it stands in for
Terraform. Pod identity does not depend on it.
[template-terraform/main.tf](template-terraform/main.tf) claims a pod from
the same pool in an ordinary Terraform template, and
[DESIGN.md](DESIGN.md) compares the ways to go from here.

## Who is trusted

- **The cluster** says which pod a token belongs to, and the Agent Sandbox
  controller says which pod a claim got. coderd believes the Kubernetes API
  and nothing a pod says about itself. Pod labels and annotations are not
  read.
- **coderd** needs a service account in the cluster that may create, read
  and delete sandbox claims in one namespace, read the sandbox and pod a
  claim leads to, and ask for token reviews. See
  [cluster/coder.yaml](cluster/coder.yaml).
- **A workload is not trusted.** It is confined to its pod. Unlike a
  workload in a daemon's sandbox, it can read its pod's token and its
  daemon's session, because they are the same principal. Both are worth
  that one workload, with its scopes, for as long as it lives.
- **The sponsor** can enter the pod with `coder ssh` and act as the
  workload there. The sponsor is the only user who can.
- **A pod that is not from a claim is nobody**, whichever service account
  it uses.

## Set up

You need a cluster with the Agent Sandbox CRDs (version 1.0 or later), and
`coder` built from this branch for the server and the CLI. Pods must be able
to reach coderd, and coderd the cluster's API.

1. Prepare the cluster. The controller here serves one namespace. With a
   cluster-wide Agent Sandbox installation, skip `controller.yaml`.

   ```sh
   kubectl apply -f cluster/namespace.yaml -f cluster/controller.yaml -f cluster/coder.yaml
   ```

2. Start coderd with the experiment and the cluster. Inside the cluster,
   run coderd as the service account `coder-server` and set only the
   namespace. Outside, give it the API server's URL, its certificate
   authority and a token of that service account.

   ```sh
   kubectl -n coder-sandbox-workloads create token coder-server --duration 720h > kube-token
   CODER_EXPERIMENTS=runtime-workloads \
   CODER_SANDBOX_KUBE_NAMESPACE=coder-sandbox-workloads \
   CODER_SANDBOX_KUBE_URL=https://<api-server> \
   CODER_SANDBOX_KUBE_CA_FILE=ca.crt \
   CODER_SANDBOX_KUBE_TOKEN_FILE=kube-token \
   coder server
   ```

3. Create the pool. Its pods download the daemon from coderd, and may reach
   only the address given here.

   ```sh
   CODER_URL=https://coder.example.com CODER_IP=<coderd's IP> CODER_PORT=443 \
     envsubst '$CODER_URL $CODER_IP $CODER_PORT' < cluster/pool.yaml | kubectl apply -f -
   ```

4. Push the template.

   ```sh
   coder templates push sandbox -d template
   ```

## Try it

```sh
coder run --template sandbox -- coder identity             # who the workload is
coder run --template sandbox -- sh -c 'echo hi; exit 3'    # output and exit code come back
coder run --template sandbox -it -- sh                     # a terminal in the pod
coder run --template sandbox --scope workspace:read --scope template:read -- coder list
coder run --template sandbox -d --name agent-1 -- sleep 600
coder run --list                                           # what runs, in every workspace
coder ssh agent-1                                          # it is a workspace
coder run --remove agent-1
coder run --all                                            # what ran, and how it ended
coder run --logs agent-1                                   # what was kept of its output
```

`--keep` leaves the workspace when the command is over. It can then be
entered, stopped and started like any workspace. Stopping gives the pod
back; starting claims another.

```sh
coder run --template sandbox --keep --name fix-1 -- make test
coder ssh fix-1
coder delete fix-1
```

`coder create --template sandbox` makes such a workspace without a
workload, in about half a second.

## Validate

```sh
CODER_URL=https://coder.example.com \
CODER_SESSION_TOKEN="$(coder tokens create --name validate --lifetime 1h)" \
python3 validate.py
```

The script runs workloads, reads their pods' tokens with `kubectl`, and
presents forged, borrowed and unclaimed tokens to coderd. It takes about
eight minutes, most of it waiting for the pool to refill between timed runs.
Its docstring lists the settings. It removes the caller's workloads and
their workspaces first, so use an account nobody else is using.

## Clean up

```sh
coder templates delete sandbox
kubectl delete namespace coder-sandbox-workloads
kubectl delete clusterrole,clusterrolebinding \
  coder-sandbox-workloads-informer-reader coder-sandbox-workloads-token-reviewer
```
