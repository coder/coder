---
title: Install Coder on K3s in an air-gapped environment
---

This tutorial helps a Coder deployment administrator install Coder on a single-node K3s cluster with no internet access.
You stage every artifact on an internet-connected workstation, carry the artifacts to the offline host on removable storage, and finish with a running workspace.

This hands-on tutorial assumes you are familiar with the principles of air-gapped computing and understand the architecture of Kubernetes clusters.
For the concepts behind an air-gapped deployment, refer to [Air-gapped deployments](../../prepare/airgap.md).
To install Coder on a cluster that has internet access, refer to [Install Coder on Kubernetes](./index.md).

## What you stage

The offline host can't download anything, so you stage each of these components on the workstation and copy it to the host.

| Component                                     | Why the host needs it                                                                     | When you need it                                                                                            |
|-----------------------------------------------|-------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------|
| K3s binary, install script, and image archive | Runs a single-node Kubernetes cluster without pulling images                              | Always                                                                                                      |
| Helm, the Coder Helm chart, and a values file | Installs and configures Coder without a connection to the chart repository                | Always                                                                                                      |
| Coder image with a Terraform provider mirror  | Builds templates without the public Terraform registry                                    | Always                                                                                                      |
| PostgreSQL image and manifest                 | Runs the database that Coder needs, inside the cluster                                    | For this tutorial. A production deployment uses an [external PostgreSQL database](../external-database.md). |
| Workspace image with code-server              | Provides the editor that the starter template otherwise downloads when a workspace starts | When your template uses code-server                                                                         |
| Edited Kubernetes starter template            | Uses the workspace image and removes the download from the startup script                 | Always                                                                                                      |

## Prerequisites

You work on two machines: an internet-connected *workstation* that stages the artifacts, and an offline *host* that runs Coder.
The host has no network connection to the workstation, so you carry the artifacts on removable storage and run the host commands on the host itself.

You need:

- A workstation with [Docker](https://docs.docker.com/engine/install/) that can build images for the host's architecture, `zstd`, and the `coder` CLI.
- An `arm64` or `amd64` host that runs Ubuntu Server 26.04 Long-Term Support (LTS), with at least 4&nbsp;GB of memory and 20&nbsp;GB of free storage, and a user who can run `sudo`.
- A host clock that shows the correct time and keeps it across restarts, because the K3s certificates depend on the clock.
  A host with no battery-backed clock and no time server on its network, such as a Raspberry Pi, loses the time when it restarts.
- A removable storage device that you can write to on the workstation and read from on the offline host, with at least 1.5&nbsp;GB of free space.

The staged artifacts total about 1.5&nbsp;GB, and the steps use about 11&nbsp;GB of storage on the host.

## Set the variables

Run the workstation commands in this tutorial in the same shell.
If you open a new shell on the workstation, set these variables again.
The host loads the variables that it needs from a file that you copy to it.

1. Set the host address.

   ```sh
   export HOST_ADDRESS=<host-address>
   ```

   Replace `<host-address>` with the host's address or host name on the offline network.
   Coder uses this address in its access URL.

1. Set the host architecture and the matching K3s binary name.

   ```sh
   export ARCH=arm64
   export K3S_BINARY=k3s-arm64
   ```

   For an `amd64` host, set `ARCH=amd64` and `K3S_BINARY=k3s`, because the K3s release names its `amd64` binary `k3s`.
   To check the architecture, run `uname -m` on the host.
   The output `aarch64` means `arm64`, and `x86_64` means `amd64`.

## Stage artifacts on the workstation

This is the only section that needs internet access on the workstation.

1. Create a directory for the artifacts and open it.

   Choose a location with at least 2&nbsp;GB of free space, and stay in this directory for the rest of the workstation steps.

   ```sh
   mkdir -p airgap-stage/coder-image airgap-stage/workspace-image
   cd airgap-stage
   ```

1. Set the versions that these steps use.

   ```sh
   export K3S_RELEASE_URL=https://github.com/k3s-io/k3s/releases/download/v1.37.2%2Bk3s1
   export CODER_VERSION=v2.38.6
   export CODER_PROVIDER_VERSION=2.19.0
   export KUBERNETES_PROVIDER_VERSION=3.5.0
   export CODE_SERVER_VERSION=4.145.0
   export HELM_VERSION=v4.3.2
   export POSTGRES_VERSION=17.11
   export WORKSPACE_IMAGE=coder-workspace:ubuntu-cs${CODE_SERVER_VERSION}
   ```

   These are the versions that the tutorial was tested with.
   To use a newer K3s release, copy its tag from the [K3s releases page](https://github.com/k3s-io/k3s/releases), and replace the `+` in the tag with `%2B` in `K3S_RELEASE_URL`.

   The Coder provider version matches the version that Coder `v2.38.6` is built against.
   To find the provider version for another Coder version, read the `github.com/coder/terraform-provider-coder` line in the `go.mod` file at that version's release tag.

1. Download the K3s binary, the K3s image archive, the checksum list, and the install script.

   ```sh
   curl -fsSLO ${K3S_RELEASE_URL}/${K3S_BINARY}
   curl -fsSLO ${K3S_RELEASE_URL}/k3s-airgap-images-${ARCH}.tar.zst
   curl -fsSLO ${K3S_RELEASE_URL}/sha256sum-${ARCH}.txt
   curl -fsSL https://get.k3s.io -o install.sh
   ```

1. Check the K3s files against the checksum list.

   ```sh
   grep -E " (${K3S_BINARY}|k3s-airgap-images-${ARCH}.tar.zst)$" sha256sum-${ARCH}.txt | shasum -a 256 -c -
   ```

   You see `OK` for both files, here for `arm64`:

   ```txt
   k3s-airgap-images-arm64.tar.zst: OK
   k3s-arm64: OK
   ```

1. Download Helm and its checksum.

   ```sh
   curl -fsSLO https://get.helm.sh/helm-${HELM_VERSION}-linux-${ARCH}.tar.gz
   curl -fsSLO https://get.helm.sh/helm-${HELM_VERSION}-linux-${ARCH}.tar.gz.sha256sum
   ```

1. Check the Helm archive against its checksum.

   ```sh
   shasum -a 256 -c helm-${HELM_VERSION}-linux-${ARCH}.tar.gz.sha256sum
   ```

   You see `OK`, here for `arm64`:

   ```txt
   helm-v4.3.2-linux-arm64.tar.gz: OK
   ```

1. Download the Coder Helm chart.

   ```sh
   curl -fsSLO https://helm.coder.com/v2/coder_helm_${CODER_VERSION#v}.tgz
   ```

1. Check the chart version.

   ```sh
   tar -xzOf coder_helm_${CODER_VERSION#v}.tgz coder/Chart.yaml | grep -E '^(appVersion|name|version):'
   ```

   You see the chart and application versions that match `CODER_VERSION`:

   ```txt
   appVersion: 2.38.6
   name: coder
   version: 2.38.6
   ```

### Build the Coder image

The Coder image needs the Terraform providers that your templates use, because the host cannot download them.
You build an image that carries a filesystem mirror of those providers.

A filesystem mirror suits a proof of concept, because it needs no other service.
For a long-lived deployment, a provider mirror on your network, such as a Terraform network mirror or an artifact repository, is easier to keep current.
Refer to [Provider installation](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-installation) in the Terraform documentation and to [Air-gapped deployments](../../prepare/airgap.md).

1. Create the Terraform CLI configuration that points to the mirror.

   ```sh
   cat > coder-image/filesystem-mirror.tfrc <<'EOF'
   provider_installation {
     filesystem_mirror {
       path = "/home/coder/.terraform.d/plugins"
     }
   }
   EOF
   ```

1. Create the Dockerfile for the Coder image.

   ```sh
   cat > coder-image/Dockerfile <<'EOF'
   ARG CODER_VERSION
   FROM ghcr.io/coder/coder:${CODER_VERSION} AS mirror
   ARG TARGETARCH
   ARG CODER_PROVIDER_VERSION
   ARG KUBERNETES_PROVIDER_VERSION
   USER root
   WORKDIR /tmp/mirror-src
   RUN printf 'terraform {\n  required_providers {\n    coder = {\n      source  = "coder/coder"\n      version = "%s"\n    }\n    kubernetes = {\n      source  = "hashicorp/kubernetes"\n      version = "%s"\n    }\n  }\n}\n' \
         "${CODER_PROVIDER_VERSION}" "${KUBERNETES_PROVIDER_VERSION}" > main.tf \
       && terraform providers mirror -platform=linux_${TARGETARCH} /tmp/plugins

   FROM ghcr.io/coder/coder:${CODER_VERSION}
   COPY --from=mirror --chown=coder:coder /tmp/plugins /home/coder/.terraform.d/plugins
   COPY --chown=coder:coder filesystem-mirror.tfrc /home/coder/.terraformrc
   ENV TF_CLI_CONFIG_FILE=/home/coder/.terraformrc
   USER coder
   EOF
   ```

   The first stage downloads the providers with the Terraform CLI that the official image already contains.
   The second stage copies the mirror into the final image.

1. Build the Coder image.

   ```sh
   docker build --platform linux/${ARCH} \
     --build-arg TARGETARCH=${ARCH} \
     --build-arg CODER_VERSION=${CODER_VERSION} \
     --build-arg CODER_PROVIDER_VERSION=${CODER_PROVIDER_VERSION} \
     --build-arg KUBERNETES_PROVIDER_VERSION=${KUBERNETES_PROVIDER_VERSION} \
     -t coder-airgap:${CODER_VERSION} coder-image
   ```

1. Check that the image contains the Coder provider.

   ```sh
   docker run --rm --platform linux/${ARCH} --entrypoint ls coder-airgap:${CODER_VERSION} \
     /home/coder/.terraform.d/plugins/registry.terraform.io/coder/coder
   ```

   You see the provider archive for your version and architecture, here for `arm64`:

   ```txt
   2.19.0.json
   index.json
   terraform-provider-coder_2.19.0_linux_arm64.zip
   ```

### Build the workspace image

The starter template installs code-server when a workspace starts, which needs internet access.
You install code-server while you build the workspace image instead.

1. Create the Dockerfile for the workspace image.

   ```sh
   cat > workspace-image/Dockerfile <<'EOF'
   FROM codercom/example-base:ubuntu
   ARG CODE_SERVER_VERSION
   USER root
   RUN curl -fsSL https://code-server.dev/install.sh \
         | sh -s -- --method=standalone --prefix=/opt/code-server --version ${CODE_SERVER_VERSION}
   USER coder
   EOF
   ```

1. Build the workspace image.

   ```sh
   docker build --platform linux/${ARCH} \
     --build-arg CODE_SERVER_VERSION=${CODE_SERVER_VERSION} \
     -t ${WORKSPACE_IMAGE} workspace-image
   ```

1. Check the code-server version in the image.

   ```sh
   docker run --rm --platform linux/${ARCH} --entrypoint /opt/code-server/bin/code-server ${WORKSPACE_IMAGE} --version | tail -1
   ```

   The output starts with your code-server version:

   ```txt
   4.145.0 ... with Code 1.145.0
   ```

### Save the images

1. Pull the PostgreSQL image.

   ```sh
   docker pull --platform linux/${ARCH} postgres:${POSTGRES_VERSION}
   ```

1. Save the three images as compressed archives.

   ```sh
   docker save coder-airgap:${CODER_VERSION} | zstd -T0 -10 -q -o coder-airgap_${CODER_VERSION}_${ARCH}.tar.zst
   docker save ${WORKSPACE_IMAGE} | zstd -T0 -10 -q -o coder-workspace_${ARCH}.tar.zst
   docker save postgres:${POSTGRES_VERSION} | zstd -T0 -10 -q -o postgres_${POSTGRES_VERSION}_${ARCH}.tar.zst
   ```

### Create the template

1. Create a copy of the Kubernetes starter template.

   ```sh
   coder templates init --id kubernetes template
   ```

   You see the extraction message:

   ```txt
   Extracting  kubernetes  to template...
   Create your template by running:
   cd template && coder templates push

   Examples provide a starting point and are expected to be edited! 🎨
   ```

1. Set the agent architecture to the host's architecture.

   ```sh
   sed -i.bak "s/arch *= \"amd64\"/arch = \"${ARCH}\"/" template/main.tf
   ```

   The starter template sets the architecture to `amd64`, so an `arm64` workspace could not run its agent.
   On an `amd64` host, the command leaves the template unchanged.

1. Use the workspace image and stop Kubernetes from pulling it.

   ```sh
   sed -i.bak \
     -e "s#codercom/example-base:ubuntu#${WORKSPACE_IMAGE}#" \
     -e 's/image_pull_policy = "Always"/image_pull_policy = "IfNotPresent"/' \
     template/main.tf
   ```

   With the `Always` pull policy, Kubernetes contacts a registry every time a workspace starts, even when the image is already on the host.

1. Create the replacement for the `startup_script` block.

   ```sh
   cat > startup-script.txt <<'EOF'
     startup_script = <<-EOT
       set -e

       /opt/code-server/bin/code-server --auth none --port 13337 \
         --disable-telemetry --disable-update-check >/tmp/code-server.log 2>&1 &
     EOT
   EOF
   ```

1. Replace the `startup_script` block in the template.

   ```sh
   perl -0pi -e 'BEGIN{local $/; open F,"startup-script.txt"; $r=<F>; close F} s/  startup_script = <<-EOT\n.*?\n  EOT\n/$r/s' template/main.tf
   ```

   The original script downloads code-server from the internet.
   The new script starts the code-server copy in the image, and its two flags stop code-server from contacting the internet for telemetry and update checks.
   This command uses Perl, which is installed on macOS and on most Linux distributions.

1. Check the changes.

   ```sh
   grep -E 'arch  *=|image  *=|image_pull_policy|code-server/bin' template/main.tf
   ```

   You see the architecture, the code-server path, your workspace image, and the pull policy, here for `arm64`:

   ```txt
     arch = "arm64"
       /opt/code-server/bin/code-server --auth none --port 13337 \
             image             = "coder-workspace:ubuntu-cs4.145.0"
             image_pull_policy = "IfNotPresent"
   ```

1. Remove the temporary files that the edits created.

   ```sh
   rm -f template/main.tf.bak startup-script.txt
   ```

### Create the configuration files

You create the long configuration files on the workstation, so that you don't type them on the host.

1. Create the PostgreSQL manifest.

   ```sh
   cat > postgres.yaml <<EOF
   apiVersion: v1
   kind: Service
   metadata:
     name: postgresql
     namespace: coder
   spec:
     selector:
       app.kubernetes.io/name: postgresql
     ports:
       - name: postgresql
         port: 5432
         targetPort: postgresql
   ---
   apiVersion: apps/v1
   kind: StatefulSet
   metadata:
     name: postgresql
     namespace: coder
   spec:
     serviceName: postgresql
     replicas: 1
     selector:
       matchLabels:
         app.kubernetes.io/name: postgresql
     template:
       metadata:
         labels:
           app.kubernetes.io/name: postgresql
       spec:
         containers:
           - name: postgresql
             image: postgres:${POSTGRES_VERSION}
             imagePullPolicy: IfNotPresent
             ports:
               - name: postgresql
                 containerPort: 5432
             env:
               - name: POSTGRES_USER
                 value: coder
               - name: POSTGRES_DB
                 value: coder
               - name: POSTGRES_PASSWORD
                 valueFrom:
                   secretKeyRef:
                     name: postgres-credentials
                     key: password
               - name: PGDATA
                 value: /var/lib/postgresql/data/pgdata
             readinessProbe:
               exec:
                 command: ["pg_isready", "-U", "coder", "-d", "coder"]
               initialDelaySeconds: 5
               periodSeconds: 10
             resources:
               requests:
                 cpu: 100m
                 memory: 256Mi
               limits:
                 memory: 512Mi
             volumeMounts:
               - name: data
                 mountPath: /var/lib/postgresql/data
     volumeClaimTemplates:
       - metadata:
           name: data
         spec:
           accessModes: ["ReadWriteOnce"]
           resources:
             requests:
               storage: 5Gi
   EOF
   ```

   The manifest sets `imagePullPolicy: IfNotPresent` so that Kubernetes uses the image that K3s imported.

1. Create the Helm values file.

   ```sh
   cat > coder-values.yaml <<EOF
   coder:
     image:
       repo: docker.io/library/coder-airgap
       tag: ${CODER_VERSION}
       pullPolicy: IfNotPresent
     resources:
       requests:
         cpu: 500m
         memory: 512Mi
       limits:
         cpu: "2"
         memory: 1Gi
     service:
       type: NodePort
       httpNodePort: "30080"
     env:
       - name: CODER_PG_CONNECTION_URL
         valueFrom:
           secretKeyRef:
             name: coder-db-url
             key: url
       - name: CODER_ACCESS_URL
         value: http://${HOST_ADDRESS}:30080
       - name: CODER_TELEMETRY_ENABLE
         value: "false"
       - name: CODER_UPDATE_CHECK
         value: "false"
       - name: CODER_DERP_SERVER_STUN_ADDRESSES
         value: disable
       - name: CODER_BLOCK_DIRECT
         value: "true"
       - name: CODER_DISABLE_TEMPLATE_BUILDER
         value: "true"
       - name: CODER_OAUTH2_GITHUB_DEFAULT_PROVIDER_ENABLE
         value: "false"
   EOF
   ```

   The values make four changes:

   - The resource requests and limits replace the chart default of 2&nbsp;CPU cores and 4&nbsp;GiB of memory, which a host with 4&nbsp;GB of memory cannot schedule.
   - The `NodePort` service avoids the ports 80 and 443 that the bundled Traefik ingress controller uses.
   - The `CODER_ACCESS_URL` value must be reachable from workspaces and from browsers on the offline network.
   - The remaining environment variables turn off telemetry, the update check, Session Traversal Utilities for Network Address Translation (STUN), the default GitHub sign-in provider, and the template builder, and they send workspace connections through the control plane.

## Copy the artifacts to the host

Run the first three steps on the workstation and the rest on the offline host.

1. Save the variables that the host steps use.

   ```sh
   cat > airgap.env <<EOF
   export HOST_ADDRESS=${HOST_ADDRESS}
   export ARCH=${ARCH}
   export K3S_BINARY=${K3S_BINARY}
   export CODER_VERSION=${CODER_VERSION}
   export HELM_VERSION=${HELM_VERSION}
   EOF
   ```

1. Create a checksum list for every staged file.

   ```sh
   find . -type f ! -name SHA256SUMS -print0 | xargs -0 shasum -a 256 > SHA256SUMS
   ```

1. Copy the staged files to removable storage.

   ```sh
   mkdir -p <removable-storage>/airgap
   cp -r ./* <removable-storage>/airgap/
   ```

   Replace `<removable-storage>` with the path where the workstation mounts the storage device, such as `/Volumes/<name>` on macOS.
   The copy of about 1.5&nbsp;GB takes a few minutes.
   When it finishes, eject the storage device.

1. On the offline host, insert and mount the removable storage.

   If the host doesn't mount the device automatically, find it with `lsblk`, and then mount it, for example with `sudo mount /dev/sdb1 /mnt`.

1. Copy the files from the removable storage to your home directory.

   ```sh
   cp -r <removable-storage>/airgap ~/airgap
   ```

   Replace `<removable-storage>` with the path where the host mounts the storage device, such as `/mnt`.

1. Check the copied files.

   ```sh
   cd ~/airgap && sha256sum -c SHA256SUMS
   ```

   The command prints one line for every staged file, and every line ends with `OK`.
   For example, on `arm64`:

   ```txt
   ./k3s-arm64: OK
   ./k3s-airgap-images-arm64.tar.zst: OK
   ./coder_helm_2.38.6.tgz: OK
   ```

1. Load the variables.

   ```sh
   . ./airgap.env
   ```

   Run the rest of the commands in this tutorial on the host, in the `~/airgap` directory.
   If you open a new shell on the host, run `cd ~/airgap && . ./airgap.env` again.
   After you create the cluster configuration in [Set up the command-line tools](#set-up-the-command-line-tools), also run its `export KUBECONFIG` command again.

## Install K3s

1. Check that the host has a default route.

   ```sh
   ip route show default
   ```

   K3s needs a default route to detect the node address, even when the route leads nowhere.
   If the command prints a route, skip to the next step.
   If it prints nothing, add a route through a virtual network interface:

   ```sh
   sudo ip link add dummy0 type dummy
   sudo ip link set dummy0 up
   sudo ip addr add 203.0.113.254/31 dev dummy0
   sudo ip route add default via 203.0.113.255 dev dummy0 metric 1000
   ```

   The route doesn't persist after a restart, and K3s doesn't start without it.
   After a restart, run these commands again, and K3s starts on its next retry.
   To keep the route instead, add it to the host's network configuration.

1. Install the K3s binary.

   ```sh
   sudo install -o root -g root -m 755 ~/airgap/${K3S_BINARY} /usr/local/bin/k3s
   ```

   The install script does not download the binary when you skip the download, so the binary must be in place first.

1. Copy the image archives into the K3s image directory.

   ```sh
   sudo mkdir -p /var/lib/rancher/k3s/agent/images && \
     sudo cp ~/airgap/*.tar.zst /var/lib/rancher/k3s/agent/images/
   ```

   The directory receives the K3s archive and your three image archives.
   K3s imports every archive in this directory when it starts.

1. Run the K3s install script without downloads.

   ```sh
   cd ~/airgap && sudo env INSTALL_K3S_SKIP_DOWNLOAD=true INSTALL_K3S_SKIP_SELINUX_RPM=true sh ./install.sh
   ```

   You see the script skip the download and start the service:

   ```txt
   [INFO]  Skipping k3s download and verify
   [INFO]  Skipping installation of SELinux RPM
   ...
   [INFO]  systemd: Starting k3s
   ```

1. Wait for the node to become ready.

   ```sh
   until sudo k3s kubectl get nodes 2>/dev/null | grep -q " Ready"; do sleep 5; done; sudo k3s kubectl get nodes
   ```

   K3s needs a few minutes to start, and the command prints nothing until the node is ready.
   You see one node with the `control-plane` role:

   ```txt
   NAME        STATUS   ROLES           AGE   VERSION
   <hostname>  Ready    control-plane   2m    v1.37.2+k3s1
   ```

1. Wait for K3s to import your three images.

   ```sh
   until [ "$(sudo k3s crictl images | grep -cE "coder-airgap|coder-workspace|postgres")" -ge 3 ]; do sleep 10; done; sudo k3s crictl images | grep -E "coder-airgap|coder-workspace|postgres"
   ```

   The import can take several minutes on a host with slow storage.
   You see the three images:

   ```txt
   docker.io/library/coder-airgap     v2.38.6           ...
   docker.io/library/coder-workspace  ubuntu-cs4.145.0  ...
   docker.io/library/postgres         17.11             ...
   ```

## Set up the command-line tools

1. Install Helm.

   ```sh
   tar -xzf helm-${HELM_VERSION}-linux-${ARCH}.tar.gz
   sudo install -o root -g root -m 755 linux-${ARCH}/helm /usr/local/bin/helm
   ```

1. Copy the cluster configuration to your home directory.

   ```sh
   mkdir -p ~/.kube
   sudo cat /etc/rancher/k3s/k3s.yaml > ~/.kube/config
   chmod 600 ~/.kube/config
   export KUBECONFIG=~/.kube/config
   ```

   The file contains cluster administrator credentials, so keep it private.
   Only root can read the original file, so `kubectl` and Helm read your copy through `KUBECONFIG`.

1. List the nodes without `sudo`.

   ```sh
   kubectl get nodes
   ```

   The K3s install script adds the `kubectl` command.
   You see the same node with the status `Ready`:

   ```txt
   NAME         STATUS   ROLES           AGE   VERSION
   <hostname>   Ready    control-plane   3m    v1.37.2+k3s1
   ```

## Install PostgreSQL

Coder needs a PostgreSQL database.
This tutorial runs the official PostgreSQL image in the cluster, which suits a proof of concept.

1. Create the namespace and the secrets.

   ```sh
   kubectl create namespace coder
   POSTGRES_PASSWORD=$(openssl rand -hex 16)
   kubectl create secret generic postgres-credentials -n coder --from-literal=password="${POSTGRES_PASSWORD}"
   kubectl create secret generic coder-db-url -n coder \
     --from-literal=url="postgres://coder:${POSTGRES_PASSWORD}@postgresql.coder.svc.cluster.local:5432/coder?sslmode=disable"
   ```

1. Apply the manifest and wait for the database.

   ```sh
   kubectl apply -f postgres.yaml
   kubectl rollout status statefulset/postgresql -n coder --timeout=300s
   ```

   You see the rollout finish:

   ```txt
   partitioned roll out complete: 1 new pods have been updated...
   ```

## Install Coder

1. Install Coder from the chart file.

   ```sh
   helm install coder coder_helm_${CODER_VERSION#v}.tgz \
     --namespace coder --values coder-values.yaml --wait --timeout 10m
   ```

   You see the release status `deployed`:

   ```txt
   STATUS: deployed
   ```

1. Check that the Coder pod runs.

   ```sh
   kubectl get pods -n coder
   ```

   Both pods show `1/1` under `READY` and `Running` under `STATUS`:

   ```txt
   NAME                     READY   STATUS    RESTARTS   AGE
   coder-6b48dbbc66-q8swg   1/1     Running   0          52s
   postgresql-0             1/1     Running   0          18m
   ```

## Create a template and a workspace

1. Install the Coder CLI from your deployment.

   ```sh
   curl -fsSL http://${HOST_ADDRESS}:30080/install.sh | sh
   ```

   The script downloads the CLI from Coder instead of the internet, so the CLI version matches the server.
   It uses `sudo` to install the CLI in `/usr/local/bin`.
   You see that the CLI is installed:

   ```txt
   Coder v2.38.6+db7c30f installed.
   ```

1. Create the first user and log in.

   ```sh
   coder login http://${HOST_ADDRESS}:30080 \
     --first-user-username <username> --first-user-email <email> \
     --first-user-password <password> --first-user-trial=false
   ```

   Replace `<username>`, `<email>`, and `<password>` with the values for the first user.
   If the password contains special characters, put it in single quotes.
   The first user becomes the owner of the deployment.

   You see a message that begins by confirming that you are authenticated:

   ```txt
   Welcome to Coder, <username>! You're authenticated.
   ```

   To use the Coder dashboard, open `http://<host-address>:30080` in a browser on the offline network, and sign in as this user.

1. Push the template.

   ```sh
   coder templates push kubernetes-airgap --directory template --variable namespace=coder --yes
   ```

   The Terraform that runs inside the Coder pod resolves its providers from the image's mirror, without internet access.
   You see that the template was created:

   ```txt
   The kubernetes-airgap template has been created at ...
   ```

1. Create a workspace.

   ```sh
   coder create my-workspace --template kubernetes-airgap \
     --parameter cpu=2 --parameter memory=2 --parameter home_disk_size=5 --yes
   ```

   You see that the workspace was created:

   ```txt
   The my-workspace workspace has been created at ...
   ```

   The first workspace can take several minutes to start on a host with slow storage, because K3s unpacks the image on first use.
   Later starts are much faster.

   While the workspace starts, you might see the warning `The workspace agent is having trouble connecting`.
   You might also see a `context deadline exceeded` event from `kubectl get events -n coder`, followed by one pod restart.
   Both are normal on the first start, and the workspace recovers on its own.

1. Run a command in the workspace.

   ```sh
   coder ssh my-workspace -- echo connected
   ```

   You see `connected`.
   If the agent has not connected yet, the command waits for it.
   You might also see a notice that the workspace startup scripts are still running.

## Verify the air gap

1. Check that code-server runs in the workspace.

   ```sh
   coder ssh my-workspace -- 'wget -q -O - http://localhost:13337/healthz'
   ```

   You see the health response:

   ```txt
   {"status":"expired","lastHeartbeat":0}
   ```

   The status `expired` is normal when no browser is connected.
   If the command prints nothing, code-server is still starting, so wait a few seconds and run the command again.
   To open the editor from a browser on the offline network, open the Coder dashboard, select **my-workspace**, and select **code-server**.

1. Confirm that the workspace has no internet access.

   ```sh
   coder ssh my-workspace -- 'wget -T 5 -q -O /dev/null https://example.com || echo "wget exit code: $?"'
   ```

   You see the exit code 4, which means that `wget` could not reach the server:

   ```txt
   wget exit code: 4
   ```

   A different exit code, or no `wget` error, means the workspace reached the internet.
   The result depends on your network, because the workspace can reach the internet if the host itself has a route to it.

## Learn more

- [Air-gapped deployments](../../prepare/airgap.md)
- [Install Coder on Kubernetes](./index.md)
- [Mirror the Coder Registry with JFrog Artifactory](../../prepare/registry-mirror.md)
- [K3s air-gap installation documentation](https://docs.k3s.io/installation/airgap)
