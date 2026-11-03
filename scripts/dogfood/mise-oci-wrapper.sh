#!/usr/bin/env bash
# Local-only helper: runs `mise oci ...` inside a Linux container so
# macOS and Windows developers don't need a local Linux VM or a host
# install of mise. CI runs `mise oci` directly on its Linux runner; it
# does not use this script.
#
# Builds a small Debian-based wrapper image with the mise binary on
# first invocation, then reuses it. Pinning to the same `MISE_VERSION`
# baked into `Dockerfile.base` avoids depending on jdxcode/mise Docker
# Hub publication cadence, which lags upstream GitHub releases by days.
#
# `oci build --from <ref>` requires <ref> to be a registry-resolvable
# reference; the host's local Docker daemon images are not visible
# inside the wrapper. See the Makefile comment.
#
# Honors CONTAINER_RUNTIME=docker (default) or CONTAINER_RUNTIME=container
# (Apple's `container` CLI on macOS).
set -euo pipefail

# scripts/check_mise_versions.sh checks these against the other mise pins.
MISE_VERSION="v2026.10.25"
MISE_SHA256="5d362ecc578dc7c146679b841dda949d6472cbf86ff5635db7afdca1b6b3fd87"
# Bump the -rN suffix when the Dockerfile heredoc below changes
# (mise version, apt packages, trust config, etc.) so cached wrapper
# images get rebuilt automatically.
WRAPPER_REVISION="r3"
RUNTIME="${CONTAINER_RUNTIME:-docker}"
WRAPPER_IMAGE="coderdev/mise-oci-wrapper:$MISE_VERSION-$WRAPPER_REVISION"

# Mount the repo root rather than $PWD: `make -C dogfood/coder` invokes
# the wrapper from dogfood/coder/, but the project mise.toml/mise.lock
# `mise oci build` consumes live at the repo root.
REPO_ROOT="$(git rev-parse --show-toplevel)"

platform_arg=()
if [ "$RUNTIME" = "container" ]; then
	platform_arg=(--platform linux/amd64)
fi

# Build the wrapper image on first invocation. The tag includes the
# mise version so a bump automatically invalidates the cache; the old
# image becomes orphaned and the user can prune it manually.
if ! "$RUNTIME" image inspect "$WRAPPER_IMAGE" >/dev/null 2>&1; then
	echo "[$0] Building $WRAPPER_IMAGE (first-time setup)..." >&2
	build_dir="$(mktemp -d)"
	trap 'rm -rf "$build_dir"' EXIT
	cat >"$build_dir/Dockerfile" <<DOCKERFILE
FROM debian:bookworm-slim
RUN apt-get update -qq && \\
    apt-get install -y -qq --no-install-recommends \\
      ca-certificates curl && \\
    rm -rf /var/lib/apt/lists/* && \\
    curl -sSLf "https://github.com/jdx/mise/releases/download/${MISE_VERSION}/mise-${MISE_VERSION}-linux-x64" -o /usr/local/bin/mise && \\
    echo "${MISE_SHA256}  /usr/local/bin/mise" | sha256sum -c && \\
    chmod +x /usr/local/bin/mise && \\
    install --directory --mode=0755 /etc/mise /etc/mise/conf.d && \\
    printf '[settings]\\ntrusted_config_paths = ["/src"]\\n' > /etc/mise/conf.d/00-trust.toml
DOCKERFILE
	"$RUNTIME" build ${platform_arg[@]+"${platform_arg[@]}"} -t "$WRAPPER_IMAGE" "$build_dir"
	rm -rf "$build_dir"
	trap - EXIT
fi

token_arg=()
if [ -n "${GITHUB_TOKEN:-}" ]; then
	token_arg=(-e "GITHUB_TOKEN=$GITHUB_TOKEN")
fi

# Mount ~/.docker when present so mise oci can find registry creds.
# Apple `container` CLI users without Docker Desktop won't have it;
# local builds don't push, so the skip is fine.
docker_config_arg=()
if [ -d "$HOME/.docker" ]; then
	docker_config_arg=(-v "$HOME/.docker:/root/.docker:ro")
fi

# `oci build` packages installed tools into layers, so it runs
# `mise install` first. `oci push --image-dir` uploads an existing
# layout and needs no tools.
# Single quotes are intentional: $@ expands inside the container's
# `sh -c`, not in this script.
# shellcheck disable=SC2016
inner_cmd='mise oci "$@"'
if [ "${1:-}" = build ]; then
	# shellcheck disable=SC2016
	inner_cmd='mise install --yes && mise oci "$@"'
fi

exec "$RUNTIME" run --rm ${platform_arg[@]+"${platform_arg[@]}"} \
	-v "$REPO_ROOT":/src -w /src \
	${docker_config_arg[@]+"${docker_config_arg[@]}"} \
	-e MISE_EXPERIMENTAL=1 \
	${token_arg[@]+"${token_arg[@]}"} \
	--entrypoint /bin/sh \
	"$WRAPPER_IMAGE" \
	-c "$inner_cmd" -- "$@"
