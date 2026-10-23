FROM node:22-bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl git openssh-client procps \
    && rm -rf /var/lib/apt/lists/* \
    && usermod --login coder --home /home/coder --move-home node \
    && groupmod --new-name coder node

RUN npm install --global @anthropic-ai/claude-code@2.1.309

COPY adapter /opt/native-agent/adapter
RUN cd /opt/native-agent/adapter \
    && mkdir -p plugin/.claude-plugin \
    && mv plugin-manifest.json plugin/.claude-plugin/plugin.json \
    && npm ci --omit=dev --ignore-scripts
COPY startup.mjs job-status.mjs read-auth.mjs /opt/native-agent/

ENV HOME=/home/coder \
    PATH=/home/coder/.local/bin:/usr/local/bin:/usr/bin:/bin \
    DISABLE_AUTOUPDATER=1
USER coder
WORKDIR /home/coder
