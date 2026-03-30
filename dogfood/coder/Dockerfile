# 1.93.1
FROM rust:slim@sha256:1d0000a49fb62f4fde24455f49d59c6c088af46202d65d8f455b722f7263e8f8 AS rust-utils
# Install rust helper programs
ENV CARGO_INSTALL_ROOT=/tmp/
# Use more reliable mirrors for Debian packages
RUN sed -i 's|http://deb.debian.org/debian|http://mirrors.edge.kernel.org/debian|g' /etc/apt/sources.list && \
	apt-get update || true
RUN apt-get update && apt-get install -y libssl-dev openssl pkg-config build-essential
RUN cargo install jj-cli typos-cli watchexec-cli

FROM ubuntu:jammy@sha256:ce4a593b4e323dcc3dd728e397e0a866a1bf516a1b7c31d6aa06991baec4f2e0 AS go

# Install Go manually, so that we can control the version
ARG GO_VERSION=1.25.8
ARG GO_CHECKSUM="ceb5e041bbc3893846bd1614d76cb4681c91dadee579426cf21a63f2d7e03be6"

# Boring Go is needed to build FIPS-compliant binaries.
RUN apt-get update && \
	apt-get install --yes curl && \
	curl --silent --show-error --location \
	"https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
	-o /usr/local/go.tar.gz && \
	echo "$GO_CHECKSUM /usr/local/go.tar.gz" | sha256sum -c && \
	rm -rf /var/lib/apt/lists/*

ENV PATH=$PATH:/usr/local/go/bin
ARG GOPATH="/tmp/"
# Install Go utilities.
RUN apt-get update && \
	apt-get install --yes gcc && \
	mkdir --parents /usr/local/go && \
	tar --extract --gzip --directory=/usr/local/go --file=/usr/local/go.tar.gz --strip-components=1 && \
	mkdir --parents "$GOPATH" && \
	go env -w GOSUMDB=sum.golang.org && \
	# swag for Swagger doc generation
	go install github.com/swaggo/swag/cmd/swag@v1.16.2 && \
	# goimports for updating imports
	go install golang.org/x/tools/cmd/goimports@v0.41.0 && \
	# protoc-gen-go is needed to build sysbox from source
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.30.0 && \
	# drpc support for v2
	go install storj.io/drpc/cmd/protoc-gen-go-drpc@v0.0.34 && \
	# migrate for migration support for v2
	go install github.com/golang-migrate/migrate/v4/cmd/migrate@v4.15.1 && \
	# Install the latest version of gopls for editors that support
	# the language server protocol (v0.21.0+ required for Go 1.25)
	go install golang.org/x/tools/gopls@v0.21.0 && \
	# gotestsum makes test output more readable
	go install gotest.tools/gotestsum@v1.9.0 && \
	# sqlc for Go code generation
	# Switched to coder/sqlc fork to fix ambiguous column bug, see:
	# - https://github.com/coder/sqlc/pull/1
	# - https://github.com/sqlc-dev/sqlc/pull/4159
	(CGO_ENABLED=1 go install github.com/coder/sqlc/cmd/sqlc@aab4e865a51df0c43e1839f81a9d349b41d14f05) && \
	# ruleguard for checking custom rules, without needing to run all of
	# golangci-lint. Check the go.mod in the release of golangci-lint that
	# we're using for the version of go-critic that it embeds, then check
	# the version of ruleguard in go-critic for that tag.
	go install github.com/quasilyte/go-ruleguard/cmd/ruleguard@v0.3.13 && \
	# shfmt for shell script formatting
	go install mvdan.cc/sh/v3/cmd/shfmt@v3.12.0 && \
	# nfpm is used with `make build` to make release packages
	go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.35.1 && \
	# yq v4 for processing YAML files (renamed to yq4 for scripts/lib.sh).
	go install github.com/mikefarah/yq/v4@v4.44.3 && \
	mv /tmp/bin/yq /tmp/bin/yq4 && \
	# mockgen for generating mocks (v0.6.0+ required for Go 1.25)
	go install go.uber.org/mock/mockgen@v0.6.0 && \
	# Reduce image size.
	apt-get remove --yes gcc && \
	apt-get autoremove --yes && \
	apt-get clean && \
	rm -rf /var/lib/apt/lists/* && \
	rm -rf /usr/local/go && \
	rm -rf /tmp/go/pkg && \
	rm -rf /tmp/go/src

# alpine:3.18
FROM us-docker.pkg.dev/coder-v2-images-public/public/alpine@sha256:fd032399cd767f310a1d1274e81cab9f0fd8a49b3589eba2c3420228cd45b6a7 AS proto
WORKDIR /tmp
RUN apk add curl unzip
RUN curl -L -o protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v23.4/protoc-23.4-linux-x86_64.zip && \
	unzip protoc.zip && \
	rm protoc.zip

FROM ubuntu:jammy@sha256:ce4a593b4e323dcc3dd728e397e0a866a1bf516a1b7c31d6aa06991baec4f2e0

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

# Use more reliable mirrors for Ubuntu packages
RUN sed -i 's|http://archive.ubuntu.com/ubuntu/|http://mirrors.edge.kernel.org/ubuntu/|g' /etc/apt/sources.list && \
	sed -i 's|http://security.ubuntu.com/ubuntu/|http://mirrors.edge.kernel.org/ubuntu/|g' /etc/apt/sources.list && \
	apt-get update --quiet && apt-get install --yes \
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
	tmux \
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

# Install Google Chrome directly from Google. Ubuntu 22.04 ships
# chromium-browser as a snap-only package, which does not work in
# Docker containers.
# configure-chrome-flags.sh is automatically run after dpkg operations
# by dogfood/coder/files/etc/apt/apt.conf.d/99-chrome-flags.
COPY configure-chrome-flags.sh /usr/local/bin/configure-chrome-flags.sh
RUN chmod a+x /usr/local/bin/configure-chrome-flags.sh && \
	wget -q https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb && \
	apt-get install --yes ./google-chrome-stable_current_amd64.deb && \
	rm google-chrome-stable_current_amd64.deb

# Install Rust via rustup. Using rustup ensures we get a current stable
# toolchain.
ENV RUSTUP_HOME=/usr/local/rustup \
	CARGO_HOME=/usr/local/cargo
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | \
	sh -s -- -y --default-toolchain stable --profile minimal
ENV PATH=$CARGO_HOME/bin:$PATH

# NOTE: In scripts/Dockerfile.base we specifically install Terraform version 1.14.5.
# Installing the same version here to match.
RUN wget -O /tmp/terraform.zip "https://releases.hashicorp.com/terraform/1.14.5/terraform_1.14.5_linux_amd64.zip" && \
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

ARG NVM_INSTALL_SHA=bdea8c52186c4dd12657e77e7515509cda5bf9fa5a2f0046bce749e62645076d
# Install frontend utilities
ENV NVM_DIR=/usr/local/nvm
ENV NODE_VERSION=22.19.0
RUN mkdir -p $NVM_DIR
RUN curl -o nvm_install.sh https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.0/install.sh && \
	echo "${NVM_INSTALL_SHA}  nvm_install.sh" | sha256sum -c && \
	bash nvm_install.sh && \
	rm nvm_install.sh
RUN source $NVM_DIR/nvm.sh && \
	nvm install $NODE_VERSION && \
	nvm use $NODE_VERSION
ENV PATH=$NVM_DIR/versions/node/v$NODE_VERSION/bin:$PATH
RUN corepack enable && \
	corepack prepare npm@10.8.1 --activate && \
	corepack prepare pnpm@10.14.0 --activate

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
ARG GOLANGCI_LINT_VERSION=1.64.8 \
	HELM_VERSION=3.12.0 \
	KUBECTX_VERSION=0.9.4 \
	SYFT_VERSION=1.20.0 \
	COSIGN_VERSION=2.4.3 \
	BUN_VERSION=1.2.15

RUN \
	# golangci-lint performs static code analysis for our Go code
	curl --silent --show-error --location --fail "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- --strip-components=1 "golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64/golangci-lint" && \
	# Helm is necessary for deploying Coder
	curl --silent --show-error --location --fail "https://get.helm.sh/helm-v${HELM_VERSION}-linux-amd64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- --strip-components=1 linux-amd64/helm && \
	# kubens and kubectx for managing Kubernetes namespaces and contexts
	curl --silent --show-error --location --fail "https://github.com/ahmetb/kubectx/releases/download/v${KUBECTX_VERSION}/kubectx_v${KUBECTX_VERSION}_linux_x86_64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- kubectx && \
	curl --silent --show-error --location --fail "https://github.com/ahmetb/kubectx/releases/download/v${KUBECTX_VERSION}/kubens_v${KUBECTX_VERSION}_linux_x86_64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- kubens && \
	# Anchore Syft for SBOM generation
	curl --silent --show-error --location --fail "https://github.com/anchore/syft/releases/download/v${SYFT_VERSION}/syft_${SYFT_VERSION}_linux_amd64.tar.gz" | \
	tar --extract --gzip --directory=/usr/local/bin --file=- syft && \
	# Sigstore Cosign for artifact signing and attestation
	curl --silent --show-error --location --fail --output /usr/local/bin/cosign "https://github.com/sigstore/cosign/releases/download/v${COSIGN_VERSION}/cosign-linux-amd64" && \
	chmod a=rx /usr/local/bin/cosign && \
	# Install Bun JavaScript runtime to /usr/local/bin
	curl --silent --show-error --location --fail "https://github.com/oven-sh/bun/releases/download/bun-v${BUN_VERSION}/bun-linux-x64.zip" --output /tmp/bun.zip && \
	unzip -q /tmp/bun.zip -d /tmp && \
	mv /tmp/bun-linux-x64/bun /usr/local/bin/ && \
	chmod a=rx /usr/local/bin/bun && \
	rm -rf /tmp/bun.zip /tmp/bun-linux-x64 && \
	apt-get clean && rm -rf /var/lib/apt/lists/*

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

# Override CARGO_HOME so cargo registry/cache writes go to the coder
# user's home directory instead of the root-owned /usr/local/cargo.
# The rustup-installed binaries remain on PATH via /usr/local/cargo/bin.
ENV CARGO_HOME="/home/coder/.cargo"

# This setting prevents Go from using the public checksum database for
# our module path prefixes. It is required because these are in private
# repositories that require authentication.
#
# For details, see: https://golang.org/ref/mod#private-modules
ENV GOPRIVATE="coder.com,cdr.dev,go.coder.com,github.com/cdr,github.com/coder"

# Increase memory allocation to NodeJS
ENV NODE_OPTIONS="--max-old-space-size=8192"
