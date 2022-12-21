# This is the multi-arch Dockerfile used for Coder. Since it's multi-arch and
# cross-compiled, it cannot have ANY "RUN" commands. All binaries are built
# using the go toolchain on the host and then copied into the build context by
# scripts/build_docker.sh.
FROM alpine:latest

# LABEL doesn't add any real layers so it's fine (and easier) to do it here than
# in the build script.
ARG CODER_VERSION
LABEL \
	org.opencontainers.image.title="Coder" \
	org.opencontainers.image.description="A tool for provisioning self-hosted development environments with Terraform." \
	org.opencontainers.image.url="https://github.com/coder/coder" \
	org.opencontainers.image.source="https://github.com/coder/coder" \
	org.opencontainers.image.version="$CODER_VERSION"

# Create coder group and user. We cannot use `addgroup` and `adduser` because
# they won't work if we're building the image for a different architecture.
COPY --chown=0:0 --chmod=644 group passwd /etc/
COPY --chown=1000:1000 --chmod=700 empty-dir /home/coder

# The coder binary is injected by scripts/build_docker.sh.
COPY --chown=1000:1000 --chmod=755 coder /opt/coder

USER 1000:1000
ENV HOME=/home/coder
ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/opt
WORKDIR /home/coder

ENTRYPOINT [ "/opt/coder", "server" ]
