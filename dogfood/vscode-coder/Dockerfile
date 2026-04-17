FROM node:24-slim@sha256:879b21aec4a1ad820c27ccd565e7c7ed955f24b92e6694556154f251e4bdb240

ARG DEBIAN_FRONTEND=noninteractive

# Electron/Chromium system libs are installed at startup via
# `playwright install-deps chromium` so they track the project's
# Electron version automatically.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl dbus git jq sudo openssh-server screen \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# gh CLI from releases (apt repo is unreliable, see cli/cli#6175).
RUN GH_CLI_VERSION=$(curl -s "https://api.github.com/repos/cli/cli/releases/latest" \
    | grep '"tag_name":' | sed -E 's/.*"v([^"]+)".*/\1/') && \
    curl -L "https://github.com/cli/cli/releases/download/v${GH_CLI_VERSION}/gh_${GH_CLI_VERSION}_linux_amd64.deb" -o /tmp/gh.deb && \
    dpkg -i /tmp/gh.deb && rm /tmp/gh.deb

# pnpm version is controlled by the project's packageManager field.
RUN corepack enable

RUN echo 'coder ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/nopasswd && \
    chmod 640 /etc/sudoers.d/nopasswd

# Replace the default node:24-slim 'node' user with 'coder' (uid 1000).
RUN userdel -r node && \
    useradd coder --create-home --shell=/bin/bash --uid=1000 --user-group

RUN ln -s /var/tmp/coder/coder-cli/coder /usr/local/bin/coder && \
    ln -s /var/tmp/coder/code-server/bin/code-server /usr/local/bin/code-server

RUN echo "PermitUserEnvironment yes" >> /etc/ssh/sshd_config

USER coder
