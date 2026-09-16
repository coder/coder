# Build from the repository root with a Linux/amd64 Coder binary at build/coder-sandbox-agent.
# Publish the result once, then use its immutable registry digest in sandbox.yaml.
ARG BASE_IMAGE=golang:1.26.5-bookworm
FROM ${BASE_IMAGE}

RUN apt-get update \
    && apt-get install -y --no-install-recommends bash ca-certificates git nodejs npm python3 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 1000 coder \
    && useradd --uid 1000 --gid 1000 --create-home --shell /bin/bash coder \
    && mkdir -p /workspace \
    && chown coder:coder /workspace
COPY --chmod=0755 build/coder-sandbox-agent /usr/local/bin/coder
COPY --chmod=0755 provisioner/sandbox/testhost/smoke.sh /usr/local/bin/sandbox-smoke
ENV HOME=/home/coder
USER 1000:1000
WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/coder", "agent"]
