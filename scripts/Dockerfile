# This is the multi-arch Dockerfile used for Coder. Since it's multi-arch and
# cross-compiled, it cannot have ANY "RUN" commands. All binaries are built
# using the go toolchain on the host and then copied into the build context by
# scripts/build_docker.sh.
#
# If you need RUN commands (e.g. to install tools via apk), add them to
# Dockerfile.base instead, which supports "RUN" commands.
ARG BASE_IMAGE
FROM $BASE_IMAGE

# LABEL doesn't add any real layers so it's fine (and easier) to do it here than
# in the build script.
ARG CODER_VERSION
LABEL \
	org.opencontainers.image.title="Coder" \
	org.opencontainers.image.description="A tool for provisioning self-hosted development environments with Terraform." \
	org.opencontainers.image.url="https://github.com/coder/coder" \
	org.opencontainers.image.source="https://github.com/coder/coder" \
	org.opencontainers.image.version="$CODER_VERSION"

# The coder binary is injected by scripts/build_docker.sh.
COPY --chown=1000:1000 --chmod=755 coder /opt/coder

ENTRYPOINT [ "/opt/coder", "server" ]
