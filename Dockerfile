FROM alpine

# LABEL doesn't add any real layers so it's fine (and easier) to do it here than
# in the build script.
ARG CODER_VERSION
LABEL \
	org.opencontainers.image.title="Coder" \
	org.opencontainers.image.description="A tool for provisioning self-hosted development environments with Terraform." \
	org.opencontainers.image.url="https://github.com/coder/coder" \
	org.opencontainers.image.source="https://github.com/coder/coder" \
	org.opencontainers.image.version="$CODER_VERSION" \
	org.opencontainers.image.licenses="AGPL-3.0"

# The coder binary is injected by scripts/build_docker.sh.
ADD coder /opt/coder

# Create coder group and user.
RUN addgroup -g 1000 coder && \
	adduser -D -g "" -h /home/coder -G coder -u 1000 -S -s /bin/sh coder
USER coder:coder

ENTRYPOINT [ "/opt/coder", "server" ]
