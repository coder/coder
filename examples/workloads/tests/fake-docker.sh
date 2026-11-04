#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == --host=unix:///var/run/docker.sock ]]
shift
printf '%s\n' "$*" >>"$FAKE_DOCKER_LOG"
case "$1" in
ps)
	printf '%s' "${FAKE_CONTAINERS:-}"
	;;
build)
	[[ ! -e "$WORKLOAD_HOST_DIR/config.json" ]]
	[[ ! -e "$WORKLOAD_HOST_DIR/prepared.json" ]]
	if [[ ${FAKE_BUILD_FAIL:-0} == 1 ]]; then
		exit 42
	fi
	printf 'Image build completed.\n'
	;;
image)
	[[ ${FAKE_MISSING_IMAGE:-0} != 1 ]] || exit 1
	image_id="sha256:$(printf '%064d' 0)"
	if [[ "$5" == '{{.Id}}' ]]; then
		printf '%s\n' "$image_id"
	else
		jq -cn --arg image "$image_id" \
			--arg revision "${FAKE_REVISION:-e26c1934a5f96c5209e7b054419db5f6c5c12323}" \
			--arg repository "${FAKE_REPOSITORY:-https://github.com/coder/coder.git}" \
			'{Id: $image, Config: {Labels: {"dev.coder.workload.revision": $revision, "dev.coder.workload.repository": $repository}}}'
	fi
	;;
*)
	exit 1
	;;
esac
