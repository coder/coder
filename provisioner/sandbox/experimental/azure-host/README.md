# Experimental native sandbox host on Azure

This recipe creates a dedicated Ubuntu 24.04 amd64 VM for the experimental native sandbox provisioner. An existing Coder deployment provisions the outer VM with Terraform. A private, headless Coder server inside the VM creates fresh gVisor sandboxes through containerd, with four built-in native provisioner workers. It uses the implementation in the public source commit you explicitly select.

This is an experimental host recipe under `provisioner/sandbox/experimental`, outside the supported examples gallery. The deployment-specific predecessor was exercised on an Azure B4ms VM; this public, source-commit-based recipe requires its own cloud validation. It is intended for a dedicated test subscription and administrator-controlled workloads, not a shared production sandbox service.

## Prerequisites and configuration

- An existing Coder deployment and external Terraform provisioner able to create Azure resource groups, networking, a VM, and managed disks. Configure [Azure provider authentication](https://coder.com/docs/admin/templates/extending-templates/provider-authentication) on that provisioner. The pinned [AzureRM provider](https://github.com/hashicorp/terraform-provider-azurerm/blob/v4.72.0/website/docs/index.html.markdown) accepts `ARM_*` environment authentication, including service principal, managed identity, or OIDC configurations. No Azure credential is a template variable or delivered to the VM.
- A public GitHub commit containing the experimental implementation, `cmd/coder`, and `provisioner/sandbox/testhost/{Dockerfile,smoke.sh}`. The commit must be reachable from the chosen repository. An ordinary released Coder commit without the experimental provisioner is insufficient. Review the selected source before running it as root on the host.
- Azure quota for the selected VM and two 128 GiB Premium disks. The default `Standard_B4ms` has 4 burstable vCPU and 16 GiB RAM. Its CPU credits constrain performance measurements; four provisioner workers do not imply capacity for four simultaneous benchmark workloads. Choose a suitable non-burstable SKU for sustained performance testing.
- Outbound access to the Coder deployment, GitHub, Go downloads and module proxies, Ubuntu/Debian package repositories, and Docker Hub. Provider downloads also require access from the external provisioner.

| Variable                  | Required/default                     | Purpose                                                                                                                |
|---------------------------|--------------------------------------|------------------------------------------------------------------------------------------------------------------------|
| `azure_subscription_id`   | Required                             | Nonsecret subscription UUID; authentication stays on the external provisioner.                                         |
| `coder_source_commit`     | Required                             | Full lowercase 40-character public source commit SHA. No patch is embedded.                                            |
| `coder_source_repository` | `https://github.com/coder/coder.git` | Public GitHub clone URL, including a public fork when needed. Credentials, URL parameters, and fragments are rejected. |
| `azure_location`          | `eastus`                             | Region with sufficient quota.                                                                                          |
| `azure_vm_size`           | `Standard_B4ms`                      | Host SKU; allow at least 4 vCPU and 16 GiB RAM.                                                                        |
| `provisioner_tags`        | `{}`                                 | Nonsecret workspace routing tags for Azure-capable external provisioners.                                              |

The network is scoped to this workspace: a `10.0.0.0/24` VNet, an outbound public IP, and an NSG denying all inbound traffic, including SSH and the inner HTTP port. The host uses a stable private address, `10.0.0.4`; the sandbox bridge uses `10.203.0.0/24`. Choose a different recipe configuration before first use if those networks conflict with connected infrastructure. Connect through the outer Coder agent. No managed identity is assigned to the VM by this recipe.

## Prepare and import

From this directory, run local checks and prepare a clean upload directory. `prepare.sh` copies an explicit allowlist, so local state, credentials, provider caches, logs, and variable files are not uploaded. Keep the generated directory outside the repository.

```sh
terraform fmt -check
terraform init -backend=false
terraform validate
bash -n bootstrap.sh prepare-state.sh prepare.sh
shellcheck bootstrap.sh prepare-state.sh prepare.sh
bash ./prepare.sh /tmp/native-sandbox-azure-template
```

Set `AZURE_SUBSCRIPTION_ID` and `CODER_SOURCE_COMMIT` locally to the nonsecret subscription and reviewed full commit. Set `CODER_SOURCE_REPOSITORY` to the public clone URL containing that commit, such as `https://github.com/coder/coder.git`. Log the local Coder CLI into your own deployment, then import the prepared directory. The following `cloud=azure` routing tag is an example; replace it with your provisioner's configured tag or omit both tag options when routing is unnecessary.

```sh
coder templates push native-sandbox-azure-host \
  --directory /tmp/native-sandbox-azure-template \
  --variable "azure_subscription_id=$AZURE_SUBSCRIPTION_ID" \
  --variable "coder_source_repository=$CODER_SOURCE_REPOSITORY" \
  --variable "coder_source_commit=$CODER_SOURCE_COMMIT" \
  --variable 'provisioner_tags={"cloud":"azure"}' \
  --provisioner-tag cloud=azure
```

See [template import options](https://coder.com/docs/reference/cli/templates_push) and [external provisioner routing](https://coder.com/docs/admin/provisioners). Terraform renders the cloud-init and launch templates from these variables; do not hand-edit encoded payloads. The launch payload contains only this bootstrap script and the public repository/commit JSON. The outer agent token is generated for each workspace build and delivered through a root-only cloud-init environment file. Treat Terraform state and rendered cloud-init as sensitive; do not publish them.

The required commit parameter avoids embedding a patch or a self-referential commit in this recipe. After the experimental source is public, supply that exact commit during import. A mutable branch or tag is rejected.

## Startup and validation

Create an outer workspace from the imported template. Cloud-init waits for data disk LUN10, formats only a blank disk, validates its filesystem label, and mounts it at `/var/lib/coder-sandbox-host`. It refuses unknown filesystems, conflicting mounts, and unexpected symlinks. The outer agent starts after the state mount succeeds and remains available while the root bootstrap service runs, with a 90-minute deadline.

Bootstrap verifies pinned containerd 2.2.8, gVisor release 20260907.0, CNI plugins 1.9.1, and Go 1.26.5 archives. It fetches the exact selected Git commit, verifies a clean checkout, builds the headless server and slim agent, builds and imports the curated OCI image, starts separate containerd/PostgreSQL/Coder services, and imports an inner `native-sandbox` template. Each sandbox starts fresh from a cached, unpacked immutable image with runsc systrap, overlayfs, and bridge CNI. No running workspace pool is created.

The source commit and runtime archives are pinned. OS package repositories and the Ubuntu image's `latest` version remain mutable, so a new VM/image build can produce a new image digest. The chosen digest is recorded in `/var/lib/coder-sandbox-host/image-ref`; source identity is recorded in `source-commit` and `source-repository` in the same directory.

The automatic smoke test creates one sandbox, connects through authenticated SSH, runs the committed Git/build/test smoke command, deletes the sandbox, and checks its containerd, runsc, snapshot, network, and IPAM cleanup. `success` in the status file means the whole sequence completed. A systemd `Result=success` alone is insufficient. One smoke sample is not percentile or concurrency evidence.

Inside the outer workspace:

```sh
sudo cat /var/lib/coder-sandbox-host/status
sudo systemctl --no-pager show coder-sandbox-bootstrap -p ActiveState -p SubState -p Result
sudo tail -n 30 /var/log/coder-sandbox-host-bootstrap.log
```

After bootstrap succeeds, use the private inner CLI configuration to create and remove sandboxes:

```sh
sudo /opt/coder-sandbox-host/bin/coder --global-config /var/lib/coder-sandbox-host/cli \
  create agent-demo --template native-sandbox --yes
sudo /opt/coder-sandbox-host/bin/coder --global-config /var/lib/coder-sandbox-host/cli \
  ssh agent-demo
sudo /opt/coder-sandbox-host/bin/coder --global-config /var/lib/coder-sandbox-host/cli \
  delete agent-demo --yes
```

The inner server is headless and uses private HTTP on port 3020. Database and admin credentials are generated on the host and stored with private permissions. Do not print or copy `/var/lib/coder-sandbox-host/secrets`, `cli`, native operation journals, Terraform state, or complete runtime specs into shared diagnostics. Bootstrap and service logs are private operational data. No diagnostic or benchmark runners are included in this recipe.

## Lifecycle

| Operation | Behavior                                                                                                                                    |
|-----------|---------------------------------------------------------------------------------------------------------------------------------------------|
| Start     | Create the VM and OS disk, attach retained state, and run setup plus smoke validation.                                                      |
| Stop      | Delete the VM and OS disk; retain the state disk, public IP, and network resources. Those retained resources can continue to incur charges. |
| Delete    | Delete every managed resource, including the persistent state disk and its credentials/database.                                            |

The state disk preserves PostgreSQL, native runtime journals, containerd data, source/build caches, tools, configuration, and delivered bootstrap assets. Home files and Docker's OS-disk image cache do not survive a stop. Azure requires an admin SSH key; Terraform generates one, does not output its private half, and retains it in sensitive Terraform state. Inbound SSH is denied.

Stop/delete inner sandboxes before stopping the host. A host stop destroys live processes, and the experiment does not transparently restore running sandboxes from its retained database. An uncertain current-boot create may deliberately retain an owner record until a later host boot proves the old creator cannot still run; follow the native provisioner's documented recovery procedure rather than removing those files manually. For source/template updates, use a normal outer stop/start so the new agent configuration is applied to a fresh OS while retaining state.
