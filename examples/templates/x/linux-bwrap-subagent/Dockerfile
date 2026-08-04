# Reference image for the bubblewrap nested subagent template.
#
# It carries exactly what the vetted reference driver resolves on the host
# side (bwrap, jq, a statically linked BusyBox) plus a CA bundle for the
# child's TLS trust, and it prepares the one shared project directory.
FROM codercom/enterprise-base:ubuntu

USER root

# bubblewrap builds the sandbox, jq parses the driver's protocol input, the
# static BusyBox becomes the child's entire userland, and ca-certificates
# provides the bundle the driver binds read-only into the sandbox.
#
# BusyBox must be the static build: no host library directory is mounted
# into the sandbox, so a dynamically linked BusyBox cannot execute there.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    bubblewrap \
    busybox-static \
    ca-certificates \
    jq && \
    rm -rf /var/lib/apt/lists/*

# Fail the build rather than the demo if the BusyBox the driver would resolve
# turns out to be dynamically linked, which is the failure the child agent
# would otherwise hit on first start. The lookup mirrors the driver's: PATH
# resolution, with both packaged names accepted.
RUN busybox_path="$(command -v busybox || command -v busybox-static || true)"; \
    if [ -z "$busybox_path" ]; then \
    echo 'busybox is not on PATH' >&2; \
    exit 1; \
    fi; \
    if ldd "$busybox_path" >/dev/null 2>&1; then \
    echo "$busybox_path is dynamically linked; the sandbox needs the static build" >&2; \
    exit 1; \
    fi

# The shared project directory has to exist and be owned by the workspace
# user before the parent agent handles its manifest: the shared-path policy
# resolves it during reconciliation, which is earlier than any coder_script.
# Creating it here also gives the project volume the right ownership, because
# Docker seeds an empty named volume from the image directory it covers.
RUN mkdir -p /home/coder/project && chown coder:coder /home/coder/project

USER coder
