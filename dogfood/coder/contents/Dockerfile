FROM rust:slim@sha256:9abf10cc84dfad6ace1b0aae3951dc5200f467c593394288c11db1e17bb4d349 AS rust-utils
# Install rust helper programs
# ENV CARGO_NET_GIT_FETCH_WITH_CLI=true
ENV CARGO_INSTALL_ROOT=/tmp/
RUN cargo install exa bat ripgrep typos-cli watchexec-cli && \
	# Reduce image size.
	rm -rf /usr/local/cargo/registry

FROM ubuntu:jammy@sha256:0e5e4a57c2499249aafc3b40fcd541e9a456aab7296681a3994d631587203f97 AS go

# Install Go manually, so that we can control the version
ARG GO_VERSION=1.22.8

# Boring Go is needed to build FIPS-compliant binaries.
RUN apt-get update && \
	apt-get install --yes curl && \
	curl --silent --show-error --location \
	"https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
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
	go install golang.org/x/tools/cmd/goimports@v0.1.7 && \
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
	go install golang.org/x/tools/gopls@latest && \
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
	# go-fuzz for fuzzy testing. they don't publish releases so we rely on latest.
	go install github.com/dvyukov/go-fuzz/go-fuzz@latest && \
	go install github.com/dvyukov/go-fuzz/go-fuzz-build@latest && \
	# go-releaser for building 'fat binaries' that work cross-platform
	go install github.com/goreleaser/goreleaser@v1.6.1 && \
	go install mvdan.cc/sh/v3/cmd/shfmt@v3.7.0 && \
	# nfpm is used with `make build` to make release packages
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

FROM gcr.io/coder-dev-1/alpine:3.18 as proto
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

# NOTE: In scripts/Dockerfile.base we specifically install Terraform version 1.10.5.
# Installing the same version here to match.
RUN wget -O /tmp/terraform.zip "https://releases.hashicorp.com/terraform/1.11.0/terraform_1.11.0_linux_amd64.zip" && \
	unzip /tmp/terraform.zip -d /usr/local/bin && \
	rm -f /tmp/terraform.zip && \
	chmod +x /usr/local/bin/terraform && \
	terraform --version

# Install the docker buildx component.
RUN DOCKER_BUILDX_VERSION=$(curl -s "https://api.github.com/repos/docker/buildx/releases/latest" | grep '"tag_name":' |  sed -E 's/.*"(v[^"]+)".*/\1/') && \
	mkdir -p /usr/local/lib/docker/cli-plugins && \
	curl -Lo /usr/local/lib/docker/cli-plugins/docker-buildx "https://github.com/docker/buildx/releases/download/${DOCKER_BUILDX_VERSION}/buildx-${DOCKER_BUILDX_VERSION}.linux-amd64" && \
	chmod a+x /usr/local/lib/docker/cli-plugins/docker-buildx

# See https://github.com/cli/cli/issues/6175#issuecomment-1235984381 for proof
# the apt repository is unreliable
RUN GH_CLI_VERSION=$(curl -s "https://api.github.com/repos/cli/cli/releases/latest" | grep '"tag_name":' |  sed -E 's/.*"v([^"]+)".*/\1/') && \
	curl -L https://github.com/cli/cli/releases/download/v${GH_CLI_VERSION}/gh_${GH_CLI_VERSION}_linux_amd64.deb -o gh.deb && \
	dpkg -i gh.deb && \
	rm gh.deb

# Install Lazygit
# See https://github.com/jesseduffield/lazygit#ubuntu
RUN LAZYGIT_VERSION=$(curl -s "https://api.github.com/repos/jesseduffield/lazygit/releases/latest" | grep '"tag_name":' |  sed -E 's/.*"v*([^"]+)".*/\1/') && \
	curl -Lo lazygit.tar.gz "https://github.com/jesseduffield/lazygit/releases/latest/download/lazygit_${LAZYGIT_VERSION}_Linux_x86_64.tar.gz" && \
	tar xf lazygit.tar.gz -C /usr/local/bin lazygit && \
	rm lazygit.tar.gz

# Install doctl
# See https://docs.digitalocean.com/reference/doctl/how-to/install
RUN DOCTL_VERSION=$(curl -s "https://api.github.com/repos/digitalocean/doctl/releases/latest" | grep '"tag_name":' | sed -E 's/.*"v([^"]+)".*/\1/') && \
	curl -L https://github.com/digitalocean/doctl/releases/download/v${DOCTL_VERSION}/doctl-${DOCTL_VERSION}-linux-amd64.tar.gz -o doctl.tar.gz && \
	tar xf doctl.tar.gz -C /usr/local/bin doctl && \
	rm doctl.tar.gz

# Install frontend utilities
ENV NVM_DIR=/usr/local/nvm
ENV NODE_VERSION=20.16.0
RUN mkdir -p $NVM_DIR
RUN curl https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.0/install.sh | bash
RUN source $NVM_DIR/nvm.sh && \
	nvm install $NODE_VERSION && \
	nvm use $NODE_VERSION
ENV PATH=$NVM_DIR/versions/node/v$NODE_VERSION/bin:$PATH
# Allow patch updates for npm and pnpm
RUN npm install -g npm@^10.8
RUN npm install -g pnpm@^9.6

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
	GOLANGCI_LINT_VERSION=1.55.2 \
	GRYPE_VERSION=0.61.1 \
	HELM_VERSION=3.12.0 \
	KUBE_LINTER_VERSION=0.6.3 \
	KUBECTX_VERSION=0.9.4 \
	STRIPE_VERSION=1.14.5 \
	TERRAGRUNT_VERSION=0.45.11 \
	TRIVY_VERSION=0.41.0

# cloud_sql_proxy, for connecting to cloudsql instances
# the upstream go.mod prevents this from being installed with go install
RUN curl --silent --show-error --location --output /usr/local/bin/cloud_sql_proxy "https://storage.googleapis.com/cloud-sql-connectors/cloud-sql-proxy/v${CLOUD_SQL_PROXY_VERSION}/cloud-sql-proxy.linux.amd64" && \
	chmod a=rx /usr/local/bin/cloud_sql_proxy && \
	# dive for scanning image layer utilization metrics in CI
	curl --silent --show-error --location "https://github.com/wagoodman/dive/releases/download/v${DIVE_VERSION}/dive_${DIVE_VERSION}_linux_amd64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- dive && \
	# docker-credential-gcr is a Docker credential helper for pushing/pulling
	# images from Google Container Registry and Artifact Registry
	curl --silent --show-error --location "https://github.com/GoogleCloudPlatform/docker-credential-gcr/releases/download/v${DOCKER_GCR_VERSION}/docker-credential-gcr_linux_amd64-${DOCKER_GCR_VERSION}.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- docker-credential-gcr && \
	# golangci-lint performs static code analysis for our Go code
	curl --silent --show-error --location "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- --strip-components=1 "golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64/golangci-lint" && \
	# Anchore Grype for scanning container images for security issues
	curl --silent --show-error --location "https://github.com/anchore/grype/releases/download/v${GRYPE_VERSION}/grype_${GRYPE_VERSION}_linux_amd64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- grype && \
	# Helm is necessary for deploying Coder
	curl --silent --show-error --location "https://get.helm.sh/helm-v${HELM_VERSION}-linux-amd64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- --strip-components=1 linux-amd64/helm && \
	# kube-linter for linting Kubernetes objects, including those
	# that Helm generates from our charts
	curl --silent --show-error --location "https://github.com/stackrox/kube-linter/releases/download/${KUBE_LINTER_VERSION}/kube-linter-linux" --output /usr/local/bin/kube-linter && \
	# kubens and kubectx for managing Kubernetes namespaces and contexts
	curl --silent --show-error --location "https://github.com/ahmetb/kubectx/releases/download/v${KUBECTX_VERSION}/kubectx_v${KUBECTX_VERSION}_linux_x86_64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- kubectx && \
	curl --silent --show-error --location "https://github.com/ahmetb/kubectx/releases/download/v${KUBECTX_VERSION}/kubens_v${KUBECTX_VERSION}_linux_x86_64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- kubens && \
	# stripe for coder.com billing API
	curl --silent --show-error --location "https://github.com/stripe/stripe-cli/releases/download/v${STRIPE_VERSION}/stripe_${STRIPE_VERSION}_linux_x86_64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- stripe && \
	# terragrunt for running Terraform and Terragrunt files
	curl --silent --show-error --location --output /usr/local/bin/terragrunt "https://github.com/gruntwork-io/terragrunt/releases/download/v${TERRAGRUNT_VERSION}/terragrunt_linux_amd64" && \
	chmod a=rx /usr/local/bin/terragrunt && \
	# AquaSec Trivy for scanning container images for security issues
	curl --silent --show-error --location "https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-64bit.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- trivy

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
ENV CLASSPATH="/usr/local/lib/antlr-4.9.2-complete.jar:${PATH}"

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
ENV PATH="/home/coder/go/bin:${PATH}"

# This setting prevents Go from using the public checksum database for
# our module path prefixes. It is required because these are in private
# repositories that require authentication.
#
# For details, see: https://golang.org/ref/mod#private-modules
ENV GOPRIVATE="coder.com,cdr.dev,go.coder.com,github.com/cdr,github.com/coder"

# Increase memory allocation to NodeJS
ENV NODE_OPTIONS="--max-old-space-size=8192"
