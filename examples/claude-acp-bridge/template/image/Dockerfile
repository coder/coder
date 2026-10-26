FROM node:22-bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl git openssh-client procps \
    && rm -rf /var/lib/apt/lists/* \
    && usermod --login coder --home /home/coder --move-home node \
    && groupmod --new-name coder node

# The official V2 package is @opencode/cli, not the V1 opencode-ai package.
RUN npm install --global @anthropic-ai/claude-code@2.1.309 @openai/codex@0.164.0 @opencode/cli@2.0.28 \
    && test "$(opencode --version)" = "opencode v2.0.28"

COPY adapter /opt/native-agent/adapter
RUN cd /opt/native-agent/adapter \
    && mkdir -p plugin/.claude-plugin \
    && mv plugin-manifest.json plugin/.claude-plugin/plugin.json \
    && npm ci --omit=dev --ignore-scripts
COPY startup.mjs shutdown.mjs read-auth.mjs /opt/native-agent/

ENV HOME=/home/coder \
    CODEX_HOME=/home/coder/.codex \
    XDG_CONFIG_HOME=/home/coder/.config \
    XDG_DATA_HOME=/home/coder/.local/share \
    XDG_STATE_HOME=/home/coder/.local/state \
    XDG_CACHE_HOME=/home/coder/.cache \
    PATH=/home/coder/.local/bin:/usr/local/bin:/usr/bin:/bin \
    DISABLE_AUTOUPDATER=1
USER coder
WORKDIR /home/coder
