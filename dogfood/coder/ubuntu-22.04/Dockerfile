FROM ubuntu:jammy@sha256:eb29ed27b0821dca09c2e28b39135e185fc1302036427d5f4d70a41ce8fd7659

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

COPY dogfood/coder/ubuntu-22.04/files /

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
	procps \
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
COPY dogfood/coder/ubuntu-22.04/configure-chrome-flags.sh /usr/local/bin/configure-chrome-flags.sh
RUN chmod a+x /usr/local/bin/configure-chrome-flags.sh && \
	wget -q https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb && \
	apt-get install --yes ./google-chrome-stable_current_amd64.deb && \
	rm google-chrome-stable_current_amd64.deb

# Install Rust via rustup. Using rustup ensures we get a current stable
# toolchain.
ENV RUSTUP_HOME=/usr/local/rustup \
	CARGO_HOME=/usr/local/cargo
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | \
	sh -s -- -y --default-toolchain stable --profile default -c rust-src
ENV PATH=$CARGO_HOME/bin:$PATH

# Install the docker buildx component.
RUN DOCKER_BUILDX_VERSION=$(curl -s "https://api.github.com/repos/docker/buildx/releases/latest" | grep '"tag_name":' |  sed -E 's/.*"(v[^"]+)".*/\1/') && \
	mkdir -p /usr/local/lib/docker/cli-plugins && \
	curl -Lo /usr/local/lib/docker/cli-plugins/docker-buildx "https://github.com/docker/buildx/releases/download/${DOCKER_BUILDX_VERSION}/buildx-${DOCKER_BUILDX_VERSION}.linux-amd64" && \
	chmod a+x /usr/local/lib/docker/cli-plugins/docker-buildx

# GitHub CLI to /usr/bin/gh. The wrapper at files/usr/local/bin/gh
# execs this for coder external-auth fallback. Apt repo is unreliable:
# https://github.com/cli/cli/issues/6175#issuecomment-1235984381
RUN GH_CLI_VERSION=$(curl -s "https://api.github.com/repos/cli/cli/releases/latest" | grep '"tag_name":' |  sed -E 's/.*"v([^"]+)".*/\1/') && \
	curl -L https://github.com/cli/cli/releases/download/v${GH_CLI_VERSION}/gh_${GH_CLI_VERSION}_linux_amd64.deb -o gh.deb && \
	dpkg -i gh.deb && \
	rm gh.deb

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

# Add coder user and allow use of docker/sudo
RUN useradd coder \
	--create-home \
	--shell=/bin/bash \
	--groups=docker \
	--uid=1000 \
	--user-group

# Install mise. Binary at /opt/mise/bin so it survives the home
# volume mount; data dir under ~/.local/share/mise so installs ride
# along on the per-workspace home volume, matching Homebrew's pattern
# (see /home/linuxbrew volume in main.tf).
ARG MISE_VERSION=v2026.5.12 \
	MISE_SHA256=a238972a3162d710b85b28c324372e96ca4e4b486c81fe78695000d9fbc77c48 \
	MISE_INSTALL_DIR=/opt/mise/bin
RUN install --directory --owner=coder --group=coder --mode=0755 "${MISE_INSTALL_DIR}" && \
	curl --silent --show-error --location --fail \
		"https://github.com/jdx/mise/releases/download/${MISE_VERSION}/mise-${MISE_VERSION}-linux-x64" \
		--output "${MISE_INSTALL_DIR}/mise" && \
	echo "${MISE_SHA256}  ${MISE_INSTALL_DIR}/mise" | sha256sum -c && \
	chown coder:coder "${MISE_INSTALL_DIR}/mise" && \
	chmod 0755 "${MISE_INSTALL_DIR}/mise" && \
	ln --symbolic "${MISE_INSTALL_DIR}/mise" /usr/local/bin/mise && \
	test -x /usr/local/bin/mise && \
	sudo --login --user=coder /bin/bash -lc 'set -euo pipefail && mise_bin="$(readlink --canonicalize /usr/local/bin/mise)" && test -w "$(dirname "$mise_bin")" && /usr/local/bin/mise --version && /usr/local/bin/mise self-update --help >/dev/null && /usr/local/bin/mise upgrade --help >/dev/null'

# Trusted paths skip mise's per-config trust prompt for the baked-in
# system config and the coder repo when cloned at the canonical
# /home/coder/coder location. Other repos a user clones still get
# the one-time `mise trust` prompt; pre-trusting all of /home/coder
# would let any mise.toml under the home dir auto-run [hooks]/[tasks].
ENV MISE_DATA_DIR=/home/coder/.local/share/mise \
	MISE_TRUSTED_CONFIG_PATHS=/home/coder/coder:/etc/mise

# Bake the project manifest in as mise's system config and ship
# the lockfile alongside it so mise verifies download checksums
# during install. We do NOT override MISE_GLOBAL_CONFIG_FILE; that
# would re-target `mise use --global` away from the user's
# ~/.config/mise/config.toml (on the home volume) into this
# image-only path, breaking the workflow.
#
# We pre-create /etc/mise as 0755 because COPY's implicitly-created
# parent dirs inherit the --chmod, which would leave /etc/mise
# without the `x` bit and unreachable to the coder user. We also
# chown to coder so mise can write the temp lockfile it uses for
# atomic rename when updating /etc/mise/mise.lock during installs.
RUN install --directory --owner=coder --group=coder --mode=0755 /etc/mise
COPY --chown=coder:coder --chmod=0644 mise.toml /etc/mise/config.toml
COPY --chown=coder:coder --chmod=0644 mise.lock /etc/mise/mise.lock

# Pre-install tools into /opt/mise/data so they survive the home
# volume's copy-on-first-mount. MISE_SHARED_INSTALL_DIRS (set below)
# exposes them at runtime; MISE_DATA_DIR stays on the home volume.
# github_token authenticates aqua's API calls (optional secret).
RUN install --directory --owner=coder --group=coder --mode=0755 /opt/mise /opt/mise/data
RUN --mount=type=secret,id=github_token,required=false \
	gh_token="$(cat /run/secrets/github_token 2>/dev/null || true)" && \
	sudo --user=coder env \
		"MISE_DATA_DIR=/opt/mise/data" \
		"MISE_TRUSTED_CONFIG_PATHS=$MISE_TRUSTED_CONFIG_PATHS" \
		"GITHUB_TOKEN=$gh_token" \
		/usr/local/bin/mise install --yes && \
	PATH="/opt/mise/data/shims:$PATH" MISE_DATA_DIR=/opt/mise/data pnpm dlx playwright@1.47.0 install --with-deps chromium && \
	rm -rf /opt/mise/data/cache /opt/mise/data/downloads && \
	apt-get clean && rm -rf /var/lib/apt/lists/*

# Install Homebrew as the coder user so the supported Linux prefix remains
# writable after the image build.
RUN sudo --login --user=coder env NONINTERACTIVE=1 CI=1 /bin/bash -lc 'set -euo pipefail && curl --silent --show-error --location --fail https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh | /bin/bash' && \
	test -x /home/linuxbrew/.linuxbrew/bin/brew && \
	sudo --login --user=coder /bin/bash -lc '/home/linuxbrew/.linuxbrew/bin/brew --version'

# Adjust OpenSSH config
RUN echo "PermitUserEnvironment yes" >>/etc/ssh/sshd_config && \
	echo "X11Forwarding yes" >>/etc/ssh/sshd_config && \
	echo "X11UseLocalhost no" >>/etc/ssh/sshd_config

USER coder

# mise shims must lead so `command -v` and `mise doctor` resolve
# mise-managed tools ahead of Homebrew and system binaries.
ENV HOMEBREW_PREFIX="/home/linuxbrew/.linuxbrew" \
	HOMEBREW_CELLAR="/home/linuxbrew/.linuxbrew/Cellar" \
	HOMEBREW_REPOSITORY="/home/linuxbrew/.linuxbrew/Homebrew"
# Pin npm globals to a stable home dir, otherwise they land in
# mise's version-specific node bin dir which isn't on PATH.
ENV NPM_CONFIG_PREFIX="/home/coder/.npm-global"
# Baked shims trail user shims on PATH so user installs win when
# both exist.
ENV MISE_SHARED_INSTALL_DIRS="/opt/mise/data/installs"
ENV PATH="/home/coder/.npm-global/bin:${MISE_DATA_DIR}/shims:/opt/mise/data/shims:${HOMEBREW_PREFIX}/bin:${HOMEBREW_PREFIX}/sbin:/home/coder/go/bin:${PATH}"

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
