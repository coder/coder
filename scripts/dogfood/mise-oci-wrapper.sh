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

# Keep MISE_VERSION + MISE_SHA256 in lockstep with the same vars in
# .github/workflows/dogfood.yaml and dogfood/coder/ubuntu-*/Dockerfile.base.
# A `min_version` check in mise.toml catches downgrades.
MISE_VERSION="v2026.5.12"
MISE_SHA256="a238972a3162d710b85b28c324372e96ca4e4b486c81fe78695000d9fbc77c48"
# Bump the -rN suffix when the Dockerfile heredoc below changes
# (mise version, apt packages, trust config, etc.) so cached wrapper
# images get rebuilt automatically.
WRAPPER_REVISION="r2"
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
# crane (the registry client mise oci shells out to) is installed via
# mise.toml at run time, not here. Keeps the image lean and avoids
# version drift between this base layer and what mise oci uses.
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

# Mount ~/.docker when present so crane can find registry creds.
# Apple `container` CLI users without Docker Desktop won't have it;
# local builds don't push, so the skip is fine.
docker_config_arg=()
if [ -d "$HOME/.docker" ]; then
	docker_config_arg=(-v "$HOME/.docker:/root/.docker:ro")
fi

# `oci build` needs all mise tools installed so it can package them
# into layers. `oci push` needs crane on PATH (mise oci shells out to
# it). Both end up running `mise install` first; build installs every
# tool, push only crane. The `export PATH=...` exposes mise's shims
# dir so `which crane` succeeds when mise oci spawns it as a child.
# Single quotes are intentional: $HOME and $@ expand inside the
# container's `sh -c`, not in this script.
# shellcheck disable=SC2016
inner_cmd='mise oci "$@"'
case "${1:-}" in
build)
	# shellcheck disable=SC2016
	inner_cmd='mise install --yes && export PATH="$HOME/.local/share/mise/shims:$PATH" && mise oci "$@"'
	;;
push)
	# shellcheck disable=SC2016
	inner_cmd='mise install --yes crane && export PATH="$HOME/.local/share/mise/shims:$PATH" && mise oci "$@"'
	;;
esac

exec "$RUNTIME" run --rm ${platform_arg[@]+"${platform_arg[@]}"} \
	-v "$REPO_ROOT":/src -w /src \
	${docker_config_arg[@]+"${docker_config_arg[@]}"} \
	-e MISE_EXPERIMENTAL=1 \
	${token_arg[@]+"${token_arg[@]}"} \
	--entrypoint /bin/sh \
	"$WRAPPER_IMAGE" \
	-c "$inner_cmd" -- "$@"
