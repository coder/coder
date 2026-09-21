FROM codercom/example-base:ubuntu

USER root

# Pin the Node.js LTS release. The official archive includes npm and npx.
ARG NODE_VERSION=24.21.0

# Tools invoke /usr/bin/sh by absolute path. Link it when the base image
# only provides a shell somewhere else, then install Node.js.
RUN set -eu; \
    if [ ! -x /usr/bin/sh ]; then \
      if [ -x /bin/sh ]; then ln -sf /bin/sh /usr/bin/sh; \
      elif [ -x /usr/bin/bash ]; then ln -sf /usr/bin/bash /usr/bin/sh; \
      elif [ -x /bin/bash ]; then ln -sf /bin/bash /usr/bin/sh; \
      else echo "no shell available to link at /usr/bin/sh" >&2; exit 1; \
      fi; \
    fi; \
    arch="$(uname -m)"; \
    case "$arch" in \
      x86_64) node_arch=x64 ;; \
      aarch64|arm64) node_arch=arm64 ;; \
      *) echo "unsupported architecture: ${arch}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${node_arch}.tar.xz" \
      | tar -xJ -C /usr/local --strip-components=1; \
    test -x /usr/bin/sh; \
    command -v npm; \
    command -v npx

USER coder
