import { Meta, StoryObj } from "@storybook/react";
import { ChatToolInvocation } from "./ChatToolInvocation";
import {
	MockStartingWorkspace,
	MockStoppedWorkspace,
	MockStoppingWorkspace,
	MockTemplate,
	MockTemplateVersion,
	MockUser,
	MockWorkspace,
	MockWorkspaceBuild,
} from "testHelpers/entities";

const meta: Meta<typeof ChatToolInvocation> = {
	title: "pages/ChatPage/ChatToolInvocation",
	component: ChatToolInvocation,
};

export default meta;
type Story = StoryObj<typeof ChatToolInvocation>;

export const GetWorkspace: Story = {
	render: () =>
		renderInvocations(
			"coder_get_workspace",
			{
				id: MockWorkspace.id,
			},
			MockWorkspace,
		),
};

export const CreateWorkspace: Story = {
	render: () =>
		renderInvocations(
			"coder_create_workspace",
			{
				name: MockWorkspace.name,
				rich_parameters: {},
				template_version_id: MockWorkspace.template_active_version_id,
				user: MockWorkspace.owner_name,
			},
			MockWorkspace,
		),
};

export const ListWorkspaces: Story = {
	render: () =>
		renderInvocations(
			"coder_list_workspaces",
			{
				owner: "me",
			},
			[
				MockWorkspace,
				MockStoppedWorkspace,
				MockStoppingWorkspace,
				MockStartingWorkspace,
			],
		),
};

export const ListTemplates: Story = {
	render: () =>
		renderInvocations("coder_list_templates", {}, [
			{
				id: MockTemplate.id,
				name: MockTemplate.name,
				description: MockTemplate.description,
				active_version_id: MockTemplate.active_version_id,
				active_user_count: MockTemplate.active_user_count,
			},
			{
				id: "another-template",
				name: "Another Template",
				description: "A different template for testing purposes.",
				active_version_id: "v2.0",
				active_user_count: 5,
			},
		]),
};

export const TemplateVersionParameters: Story = {
	render: () =>
		renderInvocations(
			"coder_template_version_parameters",
			{
				template_version_id: MockTemplateVersion.id,
			},
			[
				{
					name: "region",
					display_name: "Region",
					description: "Select the deployment region.",
					description_plaintext: "Select the deployment region.",
					type: "string",
					mutable: false,
					default_value: "us-west-1",
					icon: "",
					options: [
						{ name: "US West", description: "", value: "us-west-1", icon: "" },
						{ name: "US East", description: "", value: "us-east-1", icon: "" },
					],
					required: true,
					ephemeral: false,
				},
				{
					name: "cpu_cores",
					display_name: "CPU Cores",
					description: "Number of CPU cores.",
					description_plaintext: "Number of CPU cores.",
					type: "number",
					mutable: true,
					default_value: "4",
					icon: "",
					options: [],
					required: false,
					ephemeral: false,
				},
			],
		),
};

export const GetAuthenticatedUser: Story = {
	render: () => renderInvocations("coder_get_authenticated_user", {}, MockUser),
};

export const CreateWorkspaceBuild: Story = {
	render: () =>
		renderInvocations(
			"coder_create_workspace_build",
			{
				workspace_id: MockWorkspace.id,
				transition: "start",
			},
			MockWorkspaceBuild,
		),
};

export const CreateTemplateVersion: Story = {
	render: () =>
		renderInvocations(
			"coder_create_template_version",
			{
				template_id: MockTemplate.id,
				file_id: "file-123",
			},
			MockTemplateVersion,
		),
};

const mockLogs = [
	"[INFO] Starting build process...",
	"[DEBUG] Reading configuration file.",
	"[WARN] Deprecated setting detected.",
	"[INFO] Applying changes...",
	"[ERROR] Failed to connect to database.",
];

export const GetWorkspaceAgentLogs: Story = {
	render: () =>
		renderInvocations(
			"coder_get_workspace_agent_logs",
			{
				workspace_agent_id: "agent-456",
			},
			mockLogs,
		),
};

export const GetWorkspaceBuildLogs: Story = {
	render: () =>
		renderInvocations(
			"coder_get_workspace_build_logs",
			{
				workspace_build_id: MockWorkspaceBuild.id,
			},
			mockLogs,
		),
};

export const GetTemplateVersionLogs: Story = {
	render: () =>
		renderInvocations(
			"coder_get_template_version_logs",
			{
				template_version_id: MockTemplateVersion.id,
			},
			mockLogs,
		),
};

export const UpdateTemplateActiveVersion: Story = {
	render: () =>
		renderInvocations(
			"coder_update_template_active_version",
			{
				template_id: MockTemplate.id,
				template_version_id: MockTemplateVersion.id,
			},
			`Successfully updated active version for template ${MockTemplate.name}.`,
		),
};

export const UploadTarFile: Story = {
	render: () =>
		renderInvocations(
			"coder_upload_tar_file",
			{
				mime_type: "application/x-tar",
				files: { "main.tf": templateTerraform, "Dockerfile": templateDockerfile },
			},
			{
				hash: "sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			},
		),
};

export const CreateTemplate: Story = {
	render: () =>
		renderInvocations(
			"coder_create_template",
			{
				name: "new-template",
			},
			MockTemplate,
		),
};

export const DeleteTemplate: Story = {
	render: () =>
		renderInvocations(
			"coder_delete_template",
			{
				template_id: MockTemplate.id,
			},
			`Successfully deleted template ${MockTemplate.name}.`,
		),
};

export const GetTemplateVersion: Story = {
	render: () =>
		renderInvocations(
			"coder_get_template_version",
			{
				template_version_id: MockTemplateVersion.id,
			},
			MockTemplateVersion,
		),
};

export const DownloadTarFile: Story = {
	render: () =>
		renderInvocations(
			"coder_download_tar_file",
			{
				file_id: "file-789",
			},
			{ "main.tf": templateTerraform, "README.md": "# My Template\n" },
		),
};

const renderInvocations = <T extends ChatToolInvocation["toolName"]>(
	toolName: T,
	args: Extract<ChatToolInvocation, { toolName: T }>["args"],
	result: Extract<
		ChatToolInvocation,
		{ toolName: T; state: "result" }
	>["result"],
	error?: string,
) => {
	return (
		<>
			<ChatToolInvocation
				toolInvocation={{
					toolCallId: "call",
					toolName,
					args: args as any,
					state: "call",
				}}
			/>
			<ChatToolInvocation
				toolInvocation={{
					toolCallId: "partial-call",
					toolName,
					args: args as any,
					state: "partial-call",
				}}
			/>
			<ChatToolInvocation
				toolInvocation={{
					toolCallId: "result",
					toolName,
					args: args as any,
					state: "result",
					result: result as any,
				}}
			/>
			<ChatToolInvocation
				toolInvocation={{
					toolCallId: "result",
					toolName,
					args: args as any,
					state: "result",
					result: {
						error: error || "Something bad happened!",
					},
				}}
			/>
		</>
	);
};

const templateDockerfile = `FROM rust:slim@sha256:9abf10cc84dfad6ace1b0aae3951dc5200f467c593394288c11db1e17bb4d349 AS rust-utils
# Install rust helper programs
# ENV CARGO_NET_GIT_FETCH_WITH_CLI=true
ENV CARGO_INSTALL_ROOT=/tmp/
RUN cargo install typos-cli watchexec-cli && \
	# Reduce image size.
	rm -rf /usr/local/cargo/registry

FROM ubuntu:jammy@sha256:0e5e4a57c2499249aafc3b40fcd541e9a456aab7296681a3994d631587203f97 AS go

# Install Go manually, so that we can control the version
ARG GO_VERSION=1.24.1

# Boring Go is needed to build FIPS-compliant binaries.
RUN apt-get update && \
	apt-get install --yes curl && \
	curl --silent --show-error --location \
	"https://go.dev/dl/go\${GO_VERSION}.linux-amd64.tar.gz" \
	-o /usr/local/go.tar.gz && \
	rm -rf /var/lib/apt/lists/*

ENV PATH=$PATH:/usr/local/go/bin
ARG GOPATH="/tmp/"
# Install Go utilities.
RUN apt-get update && \
	apt-get install --yes gcc && \
	mkdir --parents /usr/local/go && \
	tar --extract --gzip --directory=/usr/local/go --file=/usr/local/go.tar.gz --strip-components=1 && \
	mkdir --parents "$GOPATH" && \
	# moq for Go tests.
	go install github.com/matryer/moq@v0.2.3 && \
	# swag for Swagger doc generation
	go install github.com/swaggo/swag/cmd/swag@v1.7.4 && \
	# go-swagger tool to generate the go coder api client
	go install github.com/go-swagger/go-swagger/cmd/swagger@v0.28.0 && \
	# goimports for updating imports
	go install golang.org/x/tools/cmd/goimports@v0.31.0 && \
	# protoc-gen-go is needed to build sysbox from source
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.30 && \
	# drpc support for v2
	go install storj.io/drpc/cmd/protoc-gen-go-drpc@v0.0.34 && \
	# migrate for migration support for v2
	go install github.com/golang-migrate/migrate/v4/cmd/migrate@v4.15.1 && \
	# goreleaser for compiling v2 binaries
	go install github.com/goreleaser/goreleaser@v1.6.1 && \
	# Install the latest version of gopls for editors that support
	# the language server protocol
	go install golang.org/x/tools/gopls@v0.18.1 && \
	# gotestsum makes test output more readable
	go install gotest.tools/gotestsum@v1.9.0 && \
	# goveralls collects code coverage metrics from tests
	# and sends to Coveralls
	go install github.com/mattn/goveralls@v0.0.11 && \
	# kind for running Kubernetes-in-Docker, needed for tests
	go install sigs.k8s.io/kind@v0.10.0 && \
	# helm-docs generates our Helm README based on a template and the
	# charts and values files
	go install github.com/norwoodj/helm-docs/cmd/helm-docs@v1.5.0 && \
	# sqlc for Go code generation
	(CGO_ENABLED=1 go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0) && \
	# gcr-cleaner-cli used by CI to prune unused images
	go install github.com/sethvargo/gcr-cleaner/cmd/gcr-cleaner-cli@v0.5.1 && \
	# ruleguard for checking custom rules, without needing to run all of
	# golangci-lint. Check the go.mod in the release of golangci-lint that
	# we're using for the version of go-critic that it embeds, then check
	# the version of ruleguard in go-critic for that tag.
	go install github.com/quasilyte/go-ruleguard/cmd/ruleguard@v0.3.13 && \
	# go-releaser for building 'fat binaries' that work cross-platform
	go install github.com/goreleaser/goreleaser@v1.6.1 && \
	go install mvdan.cc/sh/v3/cmd/shfmt@v3.7.0 && \
	# nfpm is used with \`make build\` to make release packages
	go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.35.1 && \
	# yq v4 is used to process yaml files in coder v2. Conflicts with
	# yq v3 used in v1.
	go install github.com/mikefarah/yq/v4@v4.44.3 && \
	mv /tmp/bin/yq /tmp/bin/yq4 && \
	go install go.uber.org/mock/mockgen@v0.5.0 && \
	# Reduce image size.
	apt-get remove --yes gcc && \
	apt-get autoremove --yes && \
	apt-get clean && \
	rm -rf /var/lib/apt/lists/* && \
	rm -rf /usr/local/go && \
	rm -rf /tmp/go/pkg && \
	rm -rf /tmp/go/src

# alpine:3.18
FROM gcr.io/coder-dev-1/alpine@sha256:25fad2a32ad1f6f510e528448ae1ec69a28ef81916a004d3629874104f8a7f70 AS proto 
WORKDIR /tmp
RUN apk add curl unzip
RUN curl -L -o protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v23.4/protoc-23.4-linux-x86_64.zip && \
	unzip protoc.zip && \
	rm protoc.zip

FROM ubuntu:jammy@sha256:0e5e4a57c2499249aafc3b40fcd541e9a456aab7296681a3994d631587203f97

SHELL ["/bin/bash", "-c"]

# Install packages from apt repositories
ARG DEBIAN_FRONTEND="noninteractive"

# Updated certificates are necessary to use the teraswitch mirror.
# This must be ran before copying in configuration since the config replaces
# the default mirror with teraswitch.
# Also enable the en_US.UTF-8 locale so that we don't generate multiple locales
# and unminimize to include man pages.
RUN apt-get update && \
	apt-get install --yes ca-certificates locales && \
	echo "en_US.UTF-8 UTF-8" >> /etc/locale.gen && \
	locale-gen && \
	yes | unminimize

COPY files /

# We used to copy /etc/sudoers.d/* in from files/ but this causes issues with
# permissions and layer caching. Instead, create the file directly.
RUN mkdir -p /etc/sudoers.d && \
	echo 'coder ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/nopasswd && \
	chmod 750 /etc/sudoers.d/ && \
	chmod 640 /etc/sudoers.d/nopasswd

RUN apt-get update --quiet && apt-get install --yes \
	ansible \
	apt-transport-https \
	apt-utils \
	asciinema \
	bash \
	bash-completion \
	bat \
	bats \
	bind9-dnsutils \
	build-essential \
	ca-certificates \
	cargo \
	cmake \
	containerd.io \
	crypto-policies \
	curl \
	docker-ce \
	docker-ce-cli \
	docker-compose-plugin \
	exa \
	fd-find \
	file \
	fish \
	gettext-base \
	git \
	gnupg \
	google-cloud-sdk \
	google-cloud-sdk-datastore-emulator \
	graphviz \
	helix \
	htop \
	httpie \
	inetutils-tools \
	iproute2 \
	iputils-ping \
	iputils-tracepath \
	jq \
	kubectl \
	language-pack-en \
	less \
	libgbm-dev \
	libssl-dev \
	lsb-release \
	lsof \
	man \
	meld \
	ncdu \
	neovim \
	net-tools \
	openjdk-11-jdk-headless \
	openssh-server \
	openssl \
	packer \
	pkg-config \
	postgresql-16 \
	python3 \
	python3-pip \
	ripgrep \
	rsync \
	screen \
	shellcheck \
	strace \
	sudo \
	tcptraceroute \
	termshark \
	traceroute \
	unzip \
	vim \
	wget \
	xauth \
	zip \
	zsh \
	zstd && \
	# Delete package cache to avoid consuming space in layer
	apt-get clean && \
	# Configure FIPS-compliant policies
	update-crypto-policies --set FIPS

# NOTE: In scripts/Dockerfile.base we specifically install Terraform version 1.11.3.
# Installing the same version here to match.
RUN wget -O /tmp/terraform.zip "https://releases.hashicorp.com/terraform/1.11.3/terraform_1.11.3_linux_amd64.zip" && \
	unzip /tmp/terraform.zip -d /usr/local/bin && \
	rm -f /tmp/terraform.zip && \
	chmod +x /usr/local/bin/terraform && \
	terraform --version

# Install the docker buildx component.
RUN DOCKER_BUILDX_VERSION=$(curl -s "https://api.github.com/repos/docker/buildx/releases/latest" | grep '"tag_name":' |  sed -E 's/.*"(v[^"]+)".*/\\1/') && \
	mkdir -p /usr/local/lib/docker/cli-plugins && \
	curl -Lo /usr/local/lib/docker/cli-plugins/docker-buildx "https://github.com/docker/buildx/releases/download/\${DOCKER_BUILDX_VERSION}/buildx-\${DOCKER_BUILDX_VERSION}.linux-amd64" && \
	chmod a+x /usr/local/lib/docker/cli-plugins/docker-buildx

# See https://github.com/cli/cli/issues/6175#issuecomment-1235984381 for proof
# the apt repository is unreliable
RUN GH_CLI_VERSION=$(curl -s "https://api.github.com/repos/cli/cli/releases/latest" | grep '"tag_name":' |  sed -E 's/.*"v([^"]+)".*/\\1/') && \
	curl -L https://github.com/cli/cli/releases/download/v\${GH_CLI_VERSION}/gh_\${GH_CLI_VERSION}_linux_amd64.deb -o gh.deb && \
	dpkg -i gh.deb && \
	rm gh.deb

# Install Lazygit
# See https://github.com/jesseduffield/lazygit#ubuntu
RUN LAZYGIT_VERSION=$(curl -s "https://api.github.com/repos/jesseduffield/lazygit/releases/latest" | grep '"tag_name":' |  sed -E 's/.*"v*([^"]+)".*/\\1/') && \
	curl -Lo lazygit.tar.gz "https://github.com/jesseduffield/lazygit/releases/latest/download/lazygit_\${LAZYGIT_VERSION}_Linux_x86_64.tar.gz" && \
	tar xf lazygit.tar.gz -C /usr/local/bin lazygit && \
	rm lazygit.tar.gz

# Install doctl
# See https://docs.digitalocean.com/reference/doctl/how-to/install
RUN DOCTL_VERSION=$(curl -s "https://api.github.com/repos/digitalocean/doctl/releases/latest" | grep '"tag_name":' | sed -E 's/.*"v([^"]+)".*/\\1/') && \
	curl -L https://github.com/digitalocean/doctl/releases/download/v\${DOCTL_VERSION}/doctl-\${DOCTL_VERSION}-linux-amd64.tar.gz -o doctl.tar.gz && \
	tar xf doctl.tar.gz -C /usr/local/bin doctl && \
	rm doctl.tar.gz

ARG NVM_INSTALL_SHA=bdea8c52186c4dd12657e77e7515509cda5bf9fa5a2f0046bce749e62645076d
# Install frontend utilities
ENV NVM_DIR=/usr/local/nvm
ENV NODE_VERSION=20.16.0
RUN mkdir -p $NVM_DIR
RUN curl -o nvm_install.sh https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.0/install.sh && \
	echo "\${NVM_INSTALL_SHA}  nvm_install.sh" | sha256sum -c && \
	bash nvm_install.sh && \
	rm nvm_install.sh
RUN source $NVM_DIR/nvm.sh && \
	nvm install $NODE_VERSION && \
	nvm use $NODE_VERSION
ENV PATH=$NVM_DIR/versions/node/v$NODE_VERSION/bin:$PATH
# Allow patch updates for npm and pnpm
RUN npm install -g npm@10.8.1 --integrity=sha512-Dp1C6SvSMYQI7YHq/y2l94uvI+59Eqbu1EpuKQHQ8p16txXRuRit5gH3Lnaagk2aXDIjg/Iru9pd05bnneKgdw==
RUN npm install -g pnpm@9.15.1 --integrity=sha512-GstWXmGT7769p3JwKVBGkVDPErzHZCYudYfnHRncmKQj3/lTblfqRMSb33kP9pToPCe+X6oj1n4MAztYO+S/zw==

RUN pnpx playwright@1.47.0 install --with-deps chromium

# Ensure PostgreSQL binaries are in the users $PATH.
RUN update-alternatives --install /usr/local/bin/initdb initdb /usr/lib/postgresql/16/bin/initdb 100 && \
	update-alternatives --install /usr/local/bin/postgres postgres /usr/lib/postgresql/16/bin/postgres 100

# Create links for injected dependencies
RUN ln --symbolic /var/tmp/coder/coder-cli/coder /usr/local/bin/coder && \
	ln --symbolic /var/tmp/coder/code-server/bin/code-server /usr/local/bin/code-server

# Disable the PostgreSQL systemd service.
# Coder uses a custom timescale container to test the database instead.
RUN systemctl disable \
	postgresql

# Configure systemd services for CVMs
RUN systemctl enable \
	docker \
	ssh && \
	# Workaround for envbuilder cache probing not working unless the filesystem is modified.
	touch /tmp/.envbuilder-systemctl-enable-docker-ssh-workaround

# Install tools with published releases, where that is the
# preferred/recommended installation method.
ARG CLOUD_SQL_PROXY_VERSION=2.2.0 \
	DIVE_VERSION=0.10.0 \
	DOCKER_GCR_VERSION=2.1.8 \
	GOLANGCI_LINT_VERSION=1.64.8 \
	GRYPE_VERSION=0.61.1 \
	HELM_VERSION=3.12.0 \
	KUBE_LINTER_VERSION=0.6.3 \
	KUBECTX_VERSION=0.9.4 \
	STRIPE_VERSION=1.14.5 \
	TERRAGRUNT_VERSION=0.45.11 \
	TRIVY_VERSION=0.41.0 \
	SYFT_VERSION=1.20.0 \
	COSIGN_VERSION=2.4.3

# cloud_sql_proxy, for connecting to cloudsql instances
# the upstream go.mod prevents this from being installed with go install
RUN curl --silent --show-error --location --output /usr/local/bin/cloud_sql_proxy "https://storage.googleapis.com/cloud-sql-connectors/cloud-sql-proxy/v\${CLOUD_SQL_PROXY_VERSION}/cloud-sql-proxy.linux.amd64" && \
	chmod a=rx /usr/local/bin/cloud_sql_proxy && \
	# dive for scanning image layer utilization metrics in CI
	curl --silent --show-error --location "https://github.com/wagoodman/dive/releases/download/v\${DIVE_VERSION}/dive_\${DIVE_VERSION}_linux_amd64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- dive && \
	# docker-credential-gcr is a Docker credential helper for pushing/pulling
	# images from Google Container Registry and Artifact Registry
	curl --silent --show-error --location "https://github.com/GoogleCloudPlatform/docker-credential-gcr/releases/download/v\${DOCKER_GCR_VERSION}/docker-credential-gcr_linux_amd64-\${DOCKER_GCR_VERSION}.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- docker-credential-gcr && \
	# golangci-lint performs static code analysis for our Go code
	curl --silent --show-error --location "https://github.com/golangci/golangci-lint/releases/download/v\${GOLANGCI_LINT_VERSION}/golangci-lint-\${GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- --strip-components=1 "golangci-lint-\${GOLANGCI_LINT_VERSION}-linux-amd64/golangci-lint" && \
	# Anchore Grype for scanning container images for security issues
	curl --silent --show-error --location "https://github.com/anchore/grype/releases/download/v\${GRYPE_VERSION}/grype_\${GRYPE_VERSION}_linux_amd64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- grype && \
	# Helm is necessary for deploying Coder
	curl --silent --show-error --location "https://get.helm.sh/helm-v\${HELM_VERSION}-linux-amd64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- --strip-components=1 linux-amd64/helm && \
	# kube-linter for linting Kubernetes objects, including those
	# that Helm generates from our charts
	curl --silent --show-error --location "https://github.com/stackrox/kube-linter/releases/download/\${KUBE_LINTER_VERSION}/kube-linter-linux" --output /usr/local/bin/kube-linter && \
	# kubens and kubectx for managing Kubernetes namespaces and contexts
	curl --silent --show-error --location "https://github.com/ahmetb/kubectx/releases/download/v\${KUBECTX_VERSION}/kubectx_v\${KUBECTX_VERSION}_linux_x86_64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- kubectx && \
	curl --silent --show-error --location "https://github.com/ahmetb/kubectx/releases/download/v\${KUBECTX_VERSION}/kubens_v\${KUBECTX_VERSION}_linux_x86_64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- kubens && \
	# stripe for coder.com billing API
	curl --silent --show-error --location "https://github.com/stripe/stripe-cli/releases/download/v\${STRIPE_VERSION}/stripe_\${STRIPE_VERSION}_linux_x86_64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- stripe && \
	# terragrunt for running Terraform and Terragrunt files
	curl --silent --show-error --location --output /usr/local/bin/terragrunt "https://github.com/gruntwork-io/terragrunt/releases/download/v\${TERRAGRUNT_VERSION}/terragrunt_linux_amd64" && \
	chmod a=rx /usr/local/bin/terragrunt && \
	# AquaSec Trivy for scanning container images for security issues
	curl --silent --show-error --location "https://github.com/aquasecurity/trivy/releases/download/v\${TRIVY_VERSION}/trivy_\${TRIVY_VERSION}_Linux-64bit.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- trivy && \
	# Anchore Syft for SBOM generation
	curl --silent --show-error --location "https://github.com/anchore/syft/releases/download/v\${SYFT_VERSION}/syft_\${SYFT_VERSION}_linux_amd64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- syft && \
	# Sigstore Cosign for artifact signing and attestation
	curl --silent --show-error --location --output /usr/local/bin/cosign "https://github.com/sigstore/cosign/releases/download/v\${COSIGN_VERSION}/cosign-linux-amd64" && \
	chmod a=rx /usr/local/bin/cosign

# We use yq during "make deploy" to manually substitute out fields in
# our helm values.yaml file. See https://github.com/helm/helm/issues/3141
#
# TODO: update to 4.x, we can't do this now because it included breaking
# changes (yq w doesn't work anymore)
# RUN curl --silent --show-error --location "https://github.com/mikefarah/yq/releases/download/v4.9.0/yq_linux_amd64.tar.gz" | \
#       tar --extract --gzip --directory=/usr/local/bin --file=- ./yq_linux_amd64 && \
#     mv /usr/local/bin/yq_linux_amd64 /usr/local/bin/yq

RUN curl --silent --show-error --location --output /usr/local/bin/yq "https://github.com/mikefarah/yq/releases/download/3.3.0/yq_linux_amd64" && \
	chmod a=rx /usr/local/bin/yq

# Install GoLand.
RUN mkdir --parents /usr/local/goland && \
	curl --silent --show-error --location "https://download.jetbrains.com/go/goland-2021.2.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/goland --file=- --strip-components=1 && \
	ln --symbolic /usr/local/goland/bin/goland.sh /usr/local/bin/goland

# Install Antlrv4, needed to generate paramlang lexer/parser
RUN curl --silent --show-error --location --output /usr/local/lib/antlr-4.9.2-complete.jar "https://www.antlr.org/download/antlr-4.9.2-complete.jar"
ENV CLASSPATH="/usr/local/lib/antlr-4.9.2-complete.jar:\${PATH}"

# Add coder user and allow use of docker/sudo
RUN useradd coder \
	--create-home \
	--shell=/bin/bash \
	--groups=docker \
	--uid=1000 \
	--user-group

# Adjust OpenSSH config
RUN echo "PermitUserEnvironment yes" >>/etc/ssh/sshd_config && \
	echo "X11Forwarding yes" >>/etc/ssh/sshd_config && \
	echo "X11UseLocalhost no" >>/etc/ssh/sshd_config

# We avoid copying the extracted directory since COPY slows to minutes when there
# are a lot of small files.
COPY --from=go /usr/local/go.tar.gz /usr/local/go.tar.gz
RUN mkdir /usr/local/go && \
	tar --extract --gzip --directory=/usr/local/go --file=/usr/local/go.tar.gz --strip-components=1

ENV PATH=$PATH:/usr/local/go/bin

RUN update-alternatives --install /usr/local/bin/gofmt gofmt /usr/local/go/bin/gofmt 100

COPY --from=go /tmp/bin /usr/local/bin
COPY --from=rust-utils /tmp/bin /usr/local/bin
COPY --from=proto /tmp/bin /usr/local/bin
COPY --from=proto /tmp/include /usr/local/bin/include

USER coder

# Ensure go bins are in the 'coder' user's path. Note that no go bins are
# installed in this docker file, as they'd be mounted over by the persistent
# home volume.
ENV PATH="/home/coder/go/bin:\${PATH}"

# This setting prevents Go from using the public checksum database for
# our module path prefixes. It is required because these are in private
# repositories that require authentication.
#
# For details, see: https://golang.org/ref/mod#private-modules
ENV GOPRIVATE="coder.com,cdr.dev,go.coder.com,github.com/cdr,github.com/coder"

# Increase memory allocation to NodeJS
ENV NODE_OPTIONS="--max-old-space-size=8192"
`

const templateTerraform = `terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "2.2.0-pre0"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0.0"
    }
  }
}

locals {
  // These are cluster service addresses mapped to Tailscale nodes. Ask Dean or
  // Kyle for help.
  docker_host = {
    ""              = "tcp://dogfood-ts-cdr-dev.tailscale.svc.cluster.local:2375"
    "us-pittsburgh" = "tcp://dogfood-ts-cdr-dev.tailscale.svc.cluster.local:2375"
    // For legacy reasons, this host is labelled \`eu-helsinki\` but it's
    // actually in Germany now.
    "eu-helsinki" = "tcp://katerose-fsn-cdr-dev.tailscale.svc.cluster.local:2375"
    "ap-sydney"   = "tcp://wolfgang-syd-cdr-dev.tailscale.svc.cluster.local:2375"
    "sa-saopaulo" = "tcp://oberstein-sao-cdr-dev.tailscale.svc.cluster.local:2375"
    "za-cpt"      = "tcp://schonkopf-cpt-cdr-dev.tailscale.svc.cluster.local:2375"
  }

  repo_base_dir  = data.coder_parameter.repo_base_dir.value == "~" ? "/home/coder" : replace(data.coder_parameter.repo_base_dir.value, "/^~\\//", "/home/coder/")
  repo_dir       = replace(try(module.git-clone[0].repo_dir, ""), "/^~\\//", "/home/coder/")
  container_name = "coder-\${data.coder_workspace_owner.me.name}-\${lower(data.coder_workspace.me.name)}"
}

data "coder_parameter" "repo_base_dir" {
  type        = "string"
  name        = "Coder Repository Base Directory"
  default     = "~"
  description = "The directory specified will be created (if missing) and [coder/coder](https://github.com/coder/coder) will be automatically cloned into [base directory]/coder 🪄."
  mutable     = true
}

data "coder_parameter" "image_type" {
  type        = "string"
  name        = "Coder Image"
  default     = "codercom/oss-dogfood:latest"
  description = "The Docker image used to run your workspace. Choose between nix and non-nix images."
  option {
    icon  = "/icon/coder.svg"
    name  = "Dogfood (Default)"
    value = "codercom/oss-dogfood:latest"
  }
  option {
    icon  = "/icon/nix.svg"
    name  = "Dogfood Nix (Experimental)"
    value = "codercom/oss-dogfood-nix:latest"
  }
}

data "coder_parameter" "region" {
  type    = "string"
  name    = "Region"
  icon    = "/emojis/1f30e.png"
  default = "us-pittsburgh"
  option {
    icon  = "/emojis/1f1fa-1f1f8.png"
    name  = "Pittsburgh"
    value = "us-pittsburgh"
  }
  option {
    icon = "/emojis/1f1e9-1f1ea.png"
    name = "Falkenstein"
    // For legacy reasons, this host is labelled \`eu-helsinki\` but it's
    // actually in Germany now.
    value = "eu-helsinki"
  }
  option {
    icon  = "/emojis/1f1e6-1f1fa.png"
    name  = "Sydney"
    value = "ap-sydney"
  }
  option {
    icon  = "/emojis/1f1e7-1f1f7.png"
    name  = "São Paulo"
    value = "sa-saopaulo"
  }
  option {
    icon  = "/emojis/1f1ff-1f1e6.png"
    name  = "Cape Town"
    value = "za-cpt"
  }
}

data "coder_parameter" "res_mon_memory_threshold" {
  type        = "number"
  name        = "Memory usage threshold"
  default     = 80
  description = "The memory usage threshold used in resources monitoring to trigger notifications."
  mutable     = true
  validation {
    min = 0
    max = 100
  }
}

data "coder_parameter" "res_mon_volume_threshold" {
  type        = "number"
  name        = "Volume usage threshold"
  default     = 90
  description = "The volume usage threshold used in resources monitoring to trigger notifications."
  mutable     = true
  validation {
    min = 0
    max = 100
  }
}

data "coder_parameter" "res_mon_volume_path" {
  type        = "string"
  name        = "Volume path"
  default     = "/home/coder"
  description = "The path monitored in resources monitoring to trigger notifications."
  mutable     = true
}

provider "docker" {
  host = lookup(local.docker_host, data.coder_parameter.region.value)
}

provider "coder" {}

data "coder_external_auth" "github" {
  id = "github"
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}
data "coder_workspace_tags" "tags" {
  tags = {
    "cluster" : "dogfood-v2"
    "env" : "gke"
  }
}

module "slackme" {
  count            = data.coder_workspace.me.start_count
  source           = "dev.registry.coder.com/modules/slackme/coder"
  version          = ">= 1.0.0"
  agent_id         = coder_agent.dev.id
  auth_provider_id = "slack"
}

module "dotfiles" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/dotfiles/coder"
  version  = ">= 1.0.0"
  agent_id = coder_agent.dev.id
}

module "git-clone" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/git-clone/coder"
  version  = ">= 1.0.0"
  agent_id = coder_agent.dev.id
  url      = "https://github.com/coder/coder"
  base_dir = local.repo_base_dir
}

module "personalize" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/personalize/coder"
  version  = ">= 1.0.0"
  agent_id = coder_agent.dev.id
}

module "code-server" {
  count                   = data.coder_workspace.me.start_count
  source                  = "dev.registry.coder.com/modules/code-server/coder"
  version                 = ">= 1.0.0"
  agent_id                = coder_agent.dev.id
  folder                  = local.repo_dir
  auto_install_extensions = true
}

module "vscode-web" {
  count                   = data.coder_workspace.me.start_count
  source                  = "registry.coder.com/modules/vscode-web/coder"
  version                 = ">= 1.0.0"
  agent_id                = coder_agent.dev.id
  folder                  = local.repo_dir
  extensions              = ["github.copilot"]
  auto_install_extensions = true # will install extensions from the repos .vscode/extensions.json file
  accept_license          = true
}

module "jetbrains_gateway" {
  count          = data.coder_workspace.me.start_count
  source         = "dev.registry.coder.com/modules/jetbrains-gateway/coder"
  version        = ">= 1.0.0"
  agent_id       = coder_agent.dev.id
  agent_name     = "dev"
  folder         = local.repo_dir
  jetbrains_ides = ["GO", "WS"]
  default        = "GO"
  latest         = true
}

module "filebrowser" {
  count      = data.coder_workspace.me.start_count
  source     = "dev.registry.coder.com/modules/filebrowser/coder"
  version    = ">= 1.0.0"
  agent_id   = coder_agent.dev.id
  agent_name = "dev"
}

module "coder-login" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/coder-login/coder"
  version  = ">= 1.0.0"
  agent_id = coder_agent.dev.id
}

module "cursor" {
  count    = data.coder_workspace.me.start_count
  source   = "dev.registry.coder.com/modules/cursor/coder"
  version  = ">= 1.0.0"
  agent_id = coder_agent.dev.id
  folder   = local.repo_dir
}

module "zed" {
  count    = data.coder_workspace.me.start_count
  source   = "./zed"
  agent_id = coder_agent.dev.id
  folder   = local.repo_dir
}

resource "coder_agent" "dev" {
  arch = "amd64"
  os   = "linux"
  dir  = local.repo_dir
  env = {
    OIDC_TOKEN : data.coder_workspace_owner.me.oidc_access_token,
  }
  startup_script_behavior = "blocking"

  # The following metadata blocks are optional. They are used to display
  # information about your workspace in the dashboard. You can remove them
  # if you don't want to display any information.
  metadata {
    display_name = "CPU Usage"
    key          = "cpu_usage"
    order        = 0
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage"
    key          = "ram_usage"
    order        = 1
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "CPU Usage (Host)"
    key          = "cpu_usage_host"
    order        = 2
    script       = "coder stat cpu --host"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage (Host)"
    key          = "ram_usage_host"
    order        = 3
    script       = "coder stat mem --host"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Swap Usage (Host)"
    key          = "swap_usage_host"
    order        = 4
    script       = <<EOT
      #!/usr/bin/env bash
      echo "$(free -b | awk '/^Swap/ { printf("%.1f/%.1f", $3/1024.0/1024.0/1024.0, $2/1024.0/1024.0/1024.0) }') GiB"
    EOT
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Load Average (Host)"
    key          = "load_host"
    order        = 5
    # get load avg scaled by number of cores
    script   = <<EOT
      #!/usr/bin/env bash
      echo "\`cat /proc/loadavg | awk '{ print \$1 }'\` \`nproc\`" | awk '{ printf "%0.2f", $1/$2 }'
    EOT
    interval = 60
    timeout  = 1
  }

  metadata {
    display_name = "Disk Usage (Host)"
    key          = "disk_host"
    order        = 6
    script       = "coder stat disk --path /"
    interval     = 600
    timeout      = 10
  }

  metadata {
    display_name = "Word of the Day"
    key          = "word"
    order        = 7
    script       = <<EOT
      #!/usr/bin/env bash
      curl -o - --silent https://www.merriam-webster.com/word-of-the-day 2>&1 | awk ' $0 ~ "Word of the Day: [A-z]+" { print $5; exit }'
    EOT
    interval     = 86400
    timeout      = 5
  }

  resources_monitoring {
    memory {
      enabled   = true
      threshold = data.coder_parameter.res_mon_memory_threshold.value
    }
    volume {
      enabled   = true
      threshold = data.coder_parameter.res_mon_volume_threshold.value
      path      = data.coder_parameter.res_mon_volume_path.value
    }
  }

  startup_script = <<-EOT
    #!/usr/bin/env bash
    set -eux -o pipefail

    # Allow synchronization between scripts.
    trap 'touch /tmp/.coder-startup-script.done' EXIT

    # Start Docker service
    sudo service docker start
    # Install playwright dependencies
    # We want to use the playwright version from site/package.json
    # Check if the directory exists At workspace creation as the coder_script runs in parallel so clone might not exist yet.
    while ! [[ -f "\${local.repo_dir}/site/package.json" ]]; do
      sleep 1
    done
    cd "\${local.repo_dir}" && make clean
    cd "\${local.repo_dir}/site" && pnpm install
  EOT

  shutdown_script = <<-EOT
    #!/usr/bin/env bash
    set -eux -o pipefail

    # Stop the Docker service to prevent errors during workspace destroy.
    sudo service docker stop
  EOT
}

# Add a cost so we get some quota usage in dev.coder.com
resource "coder_metadata" "home_volume" {
  resource_id = docker_volume.home_volume.id
  daily_cost  = 1
}

resource "docker_volume" "home_volume" {
  name = "coder-\${data.coder_workspace.me.id}-home"
  # Protect the volume from being deleted due to changes in attributes.
  lifecycle {
    ignore_changes = all
  }
  # Add labels in Docker to keep track of orphan resources.
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  # This field becomes outdated if the workspace is renamed but can
  # be useful for debugging or cleaning out dangling volumes.
  labels {
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
}

data "docker_registry_image" "dogfood" {
  name = data.coder_parameter.image_type.value
}

resource "docker_image" "dogfood" {
  name = "\${data.coder_parameter.image_type.value}@\${data.docker_registry_image.dogfood.sha256_digest}"
  pull_triggers = [
    data.docker_registry_image.dogfood.sha256_digest,
    sha1(join("", [for f in fileset(path.module, "files/*") : filesha1(f)])),
    filesha1("Dockerfile"),
    filesha1("nix.hash"),
  ]
  keep_locally = true
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = docker_image.dogfood.name
  name  = local.container_name
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname = data.coder_workspace.me.name
  # Use the docker gateway if the access URL is 127.0.0.1
  entrypoint = ["sh", "-c", coder_agent.dev.init_script]
  # CPU limits are unnecessary since Docker will load balance automatically
  memory  = data.coder_workspace_owner.me.name == "code-asher" ? 65536 : 32768
  runtime = "sysbox-runc"
  # Ensure the workspace is given time to execute shutdown scripts.
  destroy_grace_seconds = 60
  stop_timeout          = 60
  stop_signal           = "SIGINT"
  env = [
    "CODER_AGENT_TOKEN=\${coder_agent.dev.token}",
    "USE_CAP_NET_ADMIN=true",
    "CODER_PROC_PRIO_MGMT=1",
    "CODER_PROC_OOM_SCORE=10",
    "CODER_PROC_NICE_SCORE=1",
    "CODER_AGENT_DEVCONTAINERS_ENABLE=1",
  ]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/coder/"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }
  capabilities {
    add = ["CAP_NET_ADMIN", "CAP_SYS_NICE"]
  }
  # Add labels in Docker to keep track of orphan resources.
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name"
    value = data.coder_workspace.me.name
  }
}

resource "coder_metadata" "container_info" {
  count       = data.coder_workspace.me.start_count
  resource_id = docker_container.workspace[0].id
  item {
    key   = "memory"
    value = docker_container.workspace[0].memory
  }
  item {
    key   = "runtime"
    value = docker_container.workspace[0].runtime
  }
  item {
    key   = "region"
    value = data.coder_parameter.region.option[index(data.coder_parameter.region.option.*.value, data.coder_parameter.region.value)].name
  }
}
`;
