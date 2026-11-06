---
title: Install Coder on K3s in an air-gapped environment
---

This tutorial shows platform engineers how to install Coder on a single-node K3s cluster that has no internet access.
You stage every artifact on a connected workstation, copy the artifacts to the host, and finish with a running workspace.

The steps were tested on a Raspberry Pi 4 Model B with 4&nbsp;GB of memory running Ubuntu Server 26.04 Long-Term Support (LTS).
The commands use `arm64` artifacts, and the steps were not tested with `amd64` artifacts.

For the concepts behind an air-gapped deployment, refer to [Air-gapped deployments](../../prepare/airgap.md).
To install Coder on a cluster that has internet access, refer to [Install Coder on Kubernetes](./index.md).

## Before you begin

You work on two machines.
The workstation has internet access and prepares everything.
The host is the machine that runs Coder, and it does not need a route to the internet.

You need:

- A workstation with [Docker](https://docs.docker.com/engine/install/) that can build `linux/arm64` images, `zstd`, [Helm](https://helm.sh/docs/intro/install/) 3.5 or later, `kubectl`, and the `coder` CLI.
- An `arm64` host that runs Ubuntu Server 26.04 LTS, with at least 4&nbsp;GB of memory and 20&nbsp;GB of free storage.
- SSH access from the workstation to the host as a user who has passwordless `sudo`.
- A network path from the workstation to the host on ports 22, 6443, and 30080.

The staged artifacts total about 1.5&nbsp;GB, and the steps use about 11&nbsp;GB of storage on the host.

## Set the host variables

Run every command in this tutorial in the same workstation shell.
If you open a new shell, set these variables again, and run the `export KUBECONFIG` command again after you create it in the "Connect to the cluster" section.

1. Set the host address and the SSH user.

   ```sh
   export HOST_ADDRESS=<host-address>
   export HOST=<user>@${HOST_ADDRESS}
   ```

   Replace `<host-address>` with the address or host name that your workstation uses to reach the host.
   Replace `<user>` with your SSH user on the host.

## Stage the artifacts on the workstation

Do this section while the workstation has internet access.

1. Create a directory for the artifacts and open it.
   Choose a location with at least 2&nbsp;GB of free space, and stay in this directory for the rest of the tutorial.

   ```sh
   mkdir -p airgap-stage/coder-image airgap-stage/workspace-image airgap-stage/debs
   cd airgap-stage
   ```

1. Set the versions that these steps use.

   ```sh
   export K3S_RELEASE_URL=https://github.com/k3s-io/k3s/releases/download/v1.37.2%2Bk3s1
   export CODER_VERSION=v2.38.6
   export CODER_PROVIDER_VERSION=2.19.0
   export KUBERNETES_PROVIDER_VERSION=3.5.0
   export CODE_SERVER_VERSION=4.145.0
   export POSTGRES_VERSION=17.11
   export WORKSPACE_IMAGE=coder-workspace:ubuntu-cs${CODE_SERVER_VERSION}
   ```

   The Coder provider version matches the version that Coder `v2.38.6` is built against.
   To find the provider version for another Coder version, read the `github.com/coder/terraform-provider-coder` line in the `go.mod` file at that version's release tag.

1. Download the K3s binary, the K3s image archive, the checksum list, and the install script.

   ```sh
   curl -fsSLO ${K3S_RELEASE_URL}/k3s-arm64
   curl -fsSLO ${K3S_RELEASE_URL}/k3s-airgap-images-arm64.tar.zst
   curl -fsSLO ${K3S_RELEASE_URL}/sha256sum-arm64.txt
   curl -fsSL https://get.k3s.io -o install.sh
   ```

1. Check the K3s files against the checksum list.

   ```sh
   grep -E ' (k3s-arm64|k3s-airgap-images-arm64.tar.zst)$' sha256sum-arm64.txt | shasum -a 256 -c -
   ```

   You see `OK` for both files:

   ```txt
   k3s-airgap-images-arm64.tar.zst: OK
   k3s-arm64: OK
   ```

1. Download the Coder Helm chart.

   ```sh
   curl -fsSLO https://helm.coder.com/v2/coder_helm_${CODER_VERSION#v}.tgz
   ```

1. Check the chart version.

   ```sh
   helm show chart coder_helm_${CODER_VERSION#v}.tgz | grep -E '^(appVersion|name|version):'
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
   docker build --platform linux/arm64 \
     --build-arg TARGETARCH=arm64 \
     --build-arg CODER_VERSION=${CODER_VERSION} \
     --build-arg CODER_PROVIDER_VERSION=${CODER_PROVIDER_VERSION} \
     --build-arg KUBERNETES_PROVIDER_VERSION=${KUBERNETES_PROVIDER_VERSION} \
     -t coder-airgap:${CODER_VERSION} coder-image
   ```

1. Check that the image contains the Coder provider.

   ```sh
   docker run --rm --platform linux/arm64 --entrypoint ls coder-airgap:${CODER_VERSION} \
     /home/coder/.terraform.d/plugins/registry.terraform.io/coder/coder
   ```

   You see the provider archive for your version:

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
   docker build --platform linux/arm64 \
     --build-arg CODE_SERVER_VERSION=${CODE_SERVER_VERSION} \
     -t ${WORKSPACE_IMAGE} workspace-image
   ```

1. Check the code-server version in the image.

   ```sh
   docker run --rm --platform linux/arm64 --entrypoint /opt/code-server/bin/code-server ${WORKSPACE_IMAGE} --version | tail -1
   ```

   The output starts with your code-server version:

   ```txt
   4.145.0 ... with Code 1.145.0
   ```

### Save the images

1. Pull the PostgreSQL image.

   ```sh
   docker pull --platform linux/arm64 postgres:${POSTGRES_VERSION}
   ```

1. Save the three images as compressed archives.

   ```sh
   docker save coder-airgap:${CODER_VERSION} | zstd -T0 -10 -q -o coder-airgap_${CODER_VERSION}_arm64.tar.zst
   docker save ${WORKSPACE_IMAGE} | zstd -T0 -10 -q -o coder-workspace_arm64.tar.zst
   docker save postgres:${POSTGRES_VERSION} | zstd -T0 -10 -q -o postgres_${POSTGRES_VERSION}_arm64.tar.zst
   ```

### Download the clock package

A host without a battery-backed clock, such as a Raspberry Pi 4, starts with the wrong time after each restart.
With no internet, nothing corrects it, and the K3s certificates depend on the clock.
The `fake-hwclock` package saves the time at shutdown and restores it at startup.

1. Download the `fake-hwclock` package for the host's operating system.

   ```sh
   docker run --name stage-debs --platform linux/arm64 ubuntu:26.04 \
     bash -c 'mkdir /out && cd /out && apt-get update -qq && apt-get download fake-hwclock'
   docker cp stage-debs:/out/. debs
   docker rm stage-debs
   ```

   The `apt-get` command prints a warning about an unsandboxed download, and the package still downloads.
   The `debs` directory now holds a file named `fake-hwclock_<version>_all.deb`.

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

1. Set the agent architecture to `arm64`.

   ```sh
   sed -i.bak 's/arch *= "amd64"/arch = "arm64"/' template/main.tf
   ```

   The starter template sets the architecture to `amd64`, so an `arm64` workspace could not run its agent.

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

   You see the architecture, the code-server path, your workspace image, and the pull policy:

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

## Copy the artifacts to the host

1. Create a checksum list for every staged file.

   ```sh
   find . -type f ! -name SHA256SUMS -print0 | xargs -0 shasum -a 256 > SHA256SUMS
   ```

1. Copy the staged files to the host.

   ```sh
   ssh ${HOST} 'mkdir -p ~/airgap'
   scp -r ./* ${HOST}:airgap/
   ```

   The copy of about 1.5&nbsp;GB takes a few minutes.

1. Check the copied files on the host.

   ```sh
   ssh ${HOST} 'cd ~/airgap && sha256sum -c SHA256SUMS'
   ```

   The command prints one line for every staged file, and every line ends with `OK`.
   For example:

   ```txt
   ./k3s-arm64: OK
   ./k3s-airgap-images-arm64.tar.zst: OK
   ./coder_helm_2.38.6.tgz: OK
   ```

## Set the host clock

1. Install the `fake-hwclock` package on the host.

   ```sh
   ssh ${HOST} 'sudo dpkg -i ~/airgap/debs/fake-hwclock_*_all.deb'
   ```

1. Set the host clock to the workstation time.

   ```sh
   ssh ${HOST} "sudo date -u -s @$(date -u +%s)"
   ```

1. Save the host clock.

   ```sh
   ssh ${HOST} 'sudo fake-hwclock save'
   ```

1. Compare the two clocks.

   ```sh
   date -u +%T; ssh ${HOST} date -u +%T
   ```

   The two times differ by only a few seconds:

   ```txt
   18:03:34
   18:03:36
   ```

A production host needs a network time source on your internal network instead of this approach.

## Install K3s

1. Install the K3s binary on the host.

   ```sh
   ssh ${HOST} 'sudo install -o root -g root -m 755 ~/airgap/k3s-arm64 /usr/local/bin/k3s'
   ```

   The install script does not download the binary when you skip the download, so the binary must be in place first.

1. Copy the image archives into the K3s image directory.

   ```sh
   ssh ${HOST} 'sudo mkdir -p /var/lib/rancher/k3s/agent/images && \
     sudo cp ~/airgap/*.tar.zst /var/lib/rancher/k3s/agent/images/'
   ```

   The directory receives the K3s archive and your three image archives.
   K3s imports every archive in this directory when it starts.

1. Run the K3s install script without downloads.

   ```sh
   ssh ${HOST} 'cd ~/airgap && sudo env INSTALL_K3S_SKIP_DOWNLOAD=true INSTALL_K3S_SKIP_SELINUX_RPM=true sh ./install.sh'
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
   ssh ${HOST} 'until sudo k3s kubectl get nodes 2>/dev/null | grep -q " Ready"; do sleep 5; done; sudo k3s kubectl get nodes'
   ```

   K3s needs a few minutes to start, and the command prints nothing until the node is ready.
   You see one node with the `control-plane` role:

   ```txt
   NAME        STATUS   ROLES           AGE   VERSION
   <hostname>  Ready    control-plane   2m    v1.37.2+k3s1
   ```

1. Wait for K3s to import your three images.

   ```sh
   ssh ${HOST} 'until [ "$(sudo k3s crictl images | grep -cE "coder-airgap|coder-workspace|postgres")" -ge 3 ]; do sleep 10; done; sudo k3s crictl images | grep -E "coder-airgap|coder-workspace|postgres"'
   ```

   The import can take several minutes on a host with slow storage.
   You see the three images:

   ```txt
   docker.io/library/coder-airgap     v2.38.6           ...
   docker.io/library/coder-workspace  ubuntu-cs4.145.0  ...
   docker.io/library/postgres         17.11             ...
   ```

## Connect to the cluster

1. Copy the cluster configuration to your workstation.

   ```sh
   ssh ${HOST} 'sudo cat /etc/rancher/k3s/k3s.yaml' | sed "s#https://127.0.0.1:6443#https://${HOST_ADDRESS}:6443#" > k3s.yaml
   chmod 600 k3s.yaml
   export KUBECONFIG=$PWD/k3s.yaml
   ```

   The file contains cluster administrator credentials, so keep it private.

1. List the nodes from your workstation.

   ```sh
   kubectl get nodes
   ```

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
   - The `CODER_ACCESS_URL` value must be reachable from your browser and from workspaces.
   - The remaining environment variables turn off telemetry, the update check, Session Traversal Utilities for Network Address Translation (STUN), the default GitHub sign-in provider, and the template builder, and they send workspace connections through the control plane.

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

1. Create the first user in a browser, or run the command in this step.

   Open `http://<host-address>:30080` in a browser and complete the form.
   Then run `coder login http://<host-address>:30080` to log in from your workstation.

   To create the user and log in with one command, run this command instead:

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

   If your `coder` CLI version differs from the Coder version that you installed, every command prints a version mismatch warning.
   The commands in this tutorial still work.

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

1. Run a command in the workspace.

   ```sh
   coder ssh my-workspace -- echo connected
   ```

   You see `connected`.
   If the agent has not connected yet, the command waits for it.
   You might also see a notice that the workspace startup scripts are still running.

## Verify the air gap

1. Open code-server in a browser, or check it from the command line.

   Open the Coder dashboard, select **my-workspace**, and select **code-server**.
   You see the code-server editor.

   To check from the command line instead, run this command:

   ```sh
   coder ssh my-workspace -- 'wget -q -O - http://localhost:13337/healthz'
   ```

   You see the health response:

   ```txt
   {"status":"expired","lastHeartbeat":0}
   ```

   The status `expired` is normal when no browser is connected.

1. Confirm that the workspace has no internet access.

   ```sh
   coder ssh my-workspace -- 'wget -T 5 -q -O /dev/null https://example.com || echo "wget exit code: $?"'
   ```

   You see the exit code 4, which means that `wget` could not reach the server:

   ```txt
   wget exit code: 4
   ```

   The result depends on your network.
   The workspace can reach the internet if the host itself has a route to it.

## Learn more

- [Air-gapped deployments](../../prepare/airgap.md)
- [Install Coder on Kubernetes](./index.md)
- [Mirror the Coder Registry with JFrog Artifactory](../../prepare/registry-mirror.md)
- [K3s air-gap installation documentation](https://docs.k3s.io/installation/airgap)
