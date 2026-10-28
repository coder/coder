#!/usr/bin/env bash
# A/B Coder Agents on the current branch against its merge base with main.
#
# Builds coderd from the baseline commit (in a temporary git worktree) and
# from the current checkout, runs Terminal-Bench 2.0 against each with Coder
# Agents, and prints the comparison. Your working tree is never touched.
#
#   scripts/harbor-coder/eval.sh                 # branch vs merge-base with main
#   scripts/harbor-coder/eval.sh --baseline main # branch vs main tip
#   scripts/harbor-coder/eval.sh --baseline HEAD # noise floor: same build twice
#
# Requires: the hash-pinned venv from install.sh on PATH, docker, go, and
# HARBOR_CODER_UPSTREAM_TOKEN (a dogfood API key; the eval coderd uses
# dogfood's AI gateway as its model upstream).
#
# Tunables (env): HARBOR_CODER_MODEL (claude-sonnet-5-5), HARBOR_ATTEMPTS (2),
# HARBOR_CONCURRENCY (5, chatd's unlicensed cap on concurrent chats),
# HARBOR_DATASET (terminal-bench@2.0), HARBOR_PORT (3000),
# HARBOR_JOBS_DIR (jobs).
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${HERE}/../.." && pwd)"
STATE="${ROOT}/.harbor-coder"
JOBS="${HARBOR_JOBS_DIR:-${ROOT}/jobs}"
PORT="${HARBOR_PORT:-3000}"
export HARBOR_CODER_MODEL="${HARBOR_CODER_MODEL:-claude-sonnet-5-5}"

BASELINE_REF=""
while [[ $# -gt 0 ]]; do
	case "$1" in
	--baseline)
		BASELINE_REF="$2"
		shift 2
		;;
	*)
		echo "unknown argument: $1" >&2
		exit 2
		;;
	esac
done

# qemu-* and build-cython-ext are broken upstream (bit-rotted apt sources and
# a git clone in the verifier). The next nine are ML training, emulation, and
# long compiles that each take 20 to 60 minutes and are not decided by the
# system prompt. The last three are security tasks Anthropic's content policy
# always refuses, so they score 0 on every arm.
EXCLUDE=(
	-x 'qemu-*' -x build-cython-ext
	-x train-fasttext -x compile-compcert -x install-windows-3.11
	-x winning-avg-corewars -x extract-moves-from-video -x mcmc-sampling-stan
	-x regex-chess -x mteb-leaderboard -x caffe-cifar-10
	-x break-filter-js-from-html -x crack-7z-hash -x filter-js-from-html
)

command -v harbor >/dev/null || {
	echo "harbor not on PATH; run scripts/harbor-coder/install.sh and add .harbor-coder/venv/bin to PATH" >&2
	exit 1
}
: "${HARBOR_CODER_UPSTREAM_TOKEN:?set HARBOR_CODER_UPSTREAM_TOKEN to a dogfood API key (coder tokens create --lifetime 8h --scope user:read_personal)}"

CANDIDATE_SHA="$(git -C "${ROOT}" rev-parse HEAD)"
CANDIDATE_LABEL="$(git -C "${ROOT}" branch --show-current)"
CANDIDATE_LABEL="${CANDIDATE_LABEL:-${CANDIDATE_SHA:0:7}}"
if [[ -z "${BASELINE_REF}" ]]; then
	BASELINE_REF="$(git -C "${ROOT}" merge-base origin/main HEAD 2>/dev/null || git -C "${ROOT}" merge-base main HEAD)"
fi
BASELINE_SHA="$(git -C "${ROOT}" rev-parse "${BASELINE_REF}")"

if [[ -n "$(git -C "${ROOT}" status --porcelain -- ':!jobs' ':!.harbor-coder')" ]]; then
	echo "note: working tree has uncommitted changes; the candidate arm builds them as-is" >&2
fi

stop_coderd() {
	if [[ -f "${STATE}/coderd.pid" ]] && kill -0 "$(cat "${STATE}/coderd.pid")" 2>/dev/null; then
		kill "$(cat "${STATE}/coderd.pid")"
		sleep 3
	fi
}

WORKTREE=""
cleanup() {
	stop_coderd
	if [[ -n "${WORKTREE}" && -d "${WORKTREE}" ]]; then
		git -C "${ROOT}" worktree remove --force "${WORKTREE}" >/dev/null 2>&1 || true
	fi
}
trap cleanup EXIT

run_arm() {
	local name="$1" src="$2" sha="$3"
	stop_coderd
	echo "==> [${name}] building and serving coderd from ${sha:0:7}"
	"${HERE}/serve-coderd.sh" "${STATE}/env" "${PORT}" "${src}"
	(
		set -a
		# shellcheck disable=SC1091
		. "${STATE}/env"
		set +a
		export PATH="${STATE}:${PATH}"
		echo "==> [${name}] ${HARBOR_DATASET:-terminal-bench@2.0}, ${HARBOR_CODER_MODEL}, ${HARBOR_ATTEMPTS:-2} attempt(s), concurrency ${HARBOR_CONCURRENCY:-5}"
		rm -rf "${JOBS:?}/${name}"
		harbor run -d "${HARBOR_DATASET:-terminal-bench@2.0}" "${EXCLUDE[@]}" \
			--agent harbor_coder.agent:CoderAgentsAgent \
			--env harbor_coder.environment:CoderEnvironment \
			-k "${HARBOR_ATTEMPTS:-2}" -n "${HARBOR_CONCURRENCY:-5}" \
			-o "${JOBS}" --job-name "${name}"
	)
	# Harbor exits 0 even when every trial raised.
	if [[ "$(find "${JOBS}/${name}" -name reward.txt -o -name reward.json | wc -l)" -eq 0 ]]; then
		echo "[${name}] no trial produced a reward; check ${JOBS}/${name}/*/result.json" >&2
		exit 1
	fi
	# Free the task images before the next arm; a full arm churns ~100GB.
	docker image prune -af >/dev/null 2>&1 || true
}

echo "baseline:  ${BASELINE_SHA:0:7} (${BASELINE_REF})"
echo "candidate: ${CANDIDATE_SHA:0:7} (${CANDIDATE_LABEL})"

WORKTREE="$(mktemp -d "${TMPDIR:-/tmp}/harbor-baseline.XXXXXX")"
git -C "${ROOT}" worktree add --detach "${WORKTREE}" "${BASELINE_SHA}" >/dev/null
run_arm baseline "${WORKTREE}" "${BASELINE_SHA}"
run_arm candidate "${ROOT}" "${CANDIDATE_SHA}"

echo
python -m harbor_coder.compare \
	--baseline "${JOBS}/baseline" --candidate "${JOBS}/candidate" \
	--labels "${BASELINE_REF}" "${CANDIDATE_LABEL}" | tee "${JOBS}/report.md"
echo
echo "report: ${JOBS}/report.md"
