FROM node:22-bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl git openssh-client procps \
    && rm -rf /var/lib/apt/lists/* \
    && usermod --login coder --home /home/coder --move-home node \
    && groupmod --new-name coder node

# Node is retained for official native CLI packaging, not a Coder broker.
# The official V2 package is @opencode/cli, not the V1 opencode-ai package.
RUN npm install --global @anthropic-ai/claude-code@2.1.309 @openai/codex@0.164.0 @opencode/cli@2.0.28 \
    && test "$(opencode --version)" = "opencode v2.0.28"

COPY startup.sh /opt/native-agent/startup.sh

# The init script downloads Coder here. This is outside the persistent home so
# replacing the container cannot overwrite an executable still running there.
RUN install -d -m 0700 -o coder -g coder /opt/coder/bin
ENV HOME=/home/coder \
    BINARY_DIR=/opt/coder/bin \
    CODER_AGENT_NATIVE_AGENTS=true \
    CODEX_HOME=/home/coder/.codex \
    XDG_CONFIG_HOME=/home/coder/.config \
    XDG_DATA_HOME=/home/coder/.local/share \
    XDG_STATE_HOME=/home/coder/.local/state \
    XDG_CACHE_HOME=/home/coder/.cache \
    PATH=/opt/coder/bin:/home/coder/.local/bin:/usr/local/bin:/usr/bin:/bin \
    DISABLE_AUTOUPDATER=1
USER coder
WORKDIR /home/coder
