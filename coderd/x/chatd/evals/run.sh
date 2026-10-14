#!/usr/bin/env bash
# Runs the chatd (Coder Agents) behavioral evals through coder/aegis. See
# README.md in this directory.
set -euo pipefail

# coder/aegis commit with the coder-agents scenario controls (coder/aegis#689).
AEGIS_REF_DEFAULT=c026b544cd430b083084c3d59bc41bca84d60bf2
AEGIS_REPO=${CHATD_EVAL_AEGIS_REPO:-https://github.com/coder/aegis.git}
TB_REPO=https://github.com/harbor-framework/terminal-bench-2-1.git
TB_COMMIT=7131e4375048a0e408a8fb404b5f499d726b695b

EVALS_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(git -C "$EVALS_DIR" rev-parse --show-toplevel)
EVALS_PATHSPEC=$(git -C "$EVALS_DIR" rev-parse --show-prefix)
EVALS_PATHSPEC=${EVALS_PATHSPEC%/}
CACHE=${CHATD_EVAL_CACHE:-$HOME/.cache/chatd-evals}
ALL_SUITES=(lifecycle tool-guidance tool-descriptions correctness terminal-bench)
DEFAULT_SUITES=(lifecycle tool-guidance tool-descriptions correctness)

usage() {
	cat <<'EOF'
Usage: run.sh [options]

  --suite NAME        lifecycle, tool-guidance, tool-descriptions, correctness,
                      terminal-bench, or all (repeatable; default: all except
                      terminal-bench)
  --case ID           run only this case (repeatable)
  --ref REF|release   coder build under test: a git ref, or release for the
                      runner image's coder 2.37.0 (default: this checkout,
                      including uncommitted changes)
  --baseline-ref REF|release
                      also run every suite on this build and compare
  --model PIN         model pin (default anthropic:claude-sonnet-5-5)
  --repeats N         trials per case and arm (default 1)
  --concurrency N     parallel trials (default min(nproc/2, 8))
  --max-usd N         budget cap per suite run
  --prompt-file PATH  add a candidate-prompt arm with this system prompt
  --prompt-mode MODE  append (default) or replace the default system prompt
  --out DIR           output directory (default CACHE/runs/<timestamp>-<pid>, where
                      CACHE is $CHATD_EVAL_CACHE or ~/.cache/chatd-evals)
  --validate-only     compile and validate the suites, then exit
  --self-check        terminal-bench: verify reference solutions, no model
  --keep              keep temporary worktrees
EOF
}

die() {
	echo "run.sh: $*" >&2
	exit 1
}

log() {
	echo "==> $*" >&2
}

suites=()
cases=()
ref=""
baseline_ref=""
model=anthropic:claude-sonnet-5-5
repeats=1
concurrency=""
max_usd=""
prompt_file=""
prompt_mode=append
out=""
validate_only=0
self_check=0
keep=0

while [[ $# -gt 0 ]]; do
	case "$1" in
	--suite)
		[[ $# -ge 2 ]] || die "--suite needs a value"
		if [[ $2 == all ]]; then
			suites+=("${ALL_SUITES[@]}")
		else
			suites+=("$2")
		fi
		shift 2
		;;
	--case)
		[[ $# -ge 2 ]] || die "--case needs a value"
		cases+=("$2")
		shift 2
		;;
	--ref)
		[[ $# -ge 2 ]] || die "--ref needs a value"
		ref=$2
		shift 2
		;;
	--baseline-ref)
		[[ $# -ge 2 ]] || die "--baseline-ref needs a value"
		baseline_ref=$2
		shift 2
		;;
	--model)
		[[ $# -ge 2 ]] || die "--model needs a value"
		model=$2
		shift 2
		;;
	--repeats)
		[[ $# -ge 2 ]] || die "--repeats needs a value"
		repeats=$2
		shift 2
		;;
	--concurrency)
		[[ $# -ge 2 ]] || die "--concurrency needs a value"
		concurrency=$2
		shift 2
		;;
	--max-usd)
		[[ $# -ge 2 ]] || die "--max-usd needs a value"
		max_usd=$2
		shift 2
		;;
	--prompt-file)
		[[ $# -ge 2 ]] || die "--prompt-file needs a value"
		[[ -f $2 ]] || die "prompt file $2 not found"
		prompt_file=$(cd "$(dirname "$2")" && pwd)/$(basename "$2")
		shift 2
		;;
	--prompt-mode)
		[[ $# -ge 2 ]] || die "--prompt-mode needs a value"
		prompt_mode=$2
		shift 2
		;;
	--out)
		[[ $# -ge 2 ]] || die "--out needs a value"
		out=$2
		shift 2
		;;
	--validate-only)
		validate_only=1
		shift
		;;
	--self-check)
		self_check=1
		shift
		;;
	--keep)
		keep=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		usage >&2
		die "unknown argument $1"
		;;
	esac
done

[[ $repeats =~ ^[1-9][0-9]*$ ]] || die "--repeats must be a positive integer"
[[ -z $concurrency || $concurrency =~ ^[1-9][0-9]*$ ]] || die "--concurrency must be a positive integer"
[[ -z $max_usd || $max_usd =~ ^[0-9]+([.][0-9]+)?$ ]] || die "--max-usd must be a number"
[[ -z $max_usd || ! $max_usd =~ ^0+([.]0+)?$ ]] || die "--max-usd must be greater than 0"
[[ $prompt_mode == append || $prompt_mode == replace ]] || die "--prompt-mode must be append or replace"
[[ $model == *:* ]] || die "--model must be provider:model, for example anthropic:claude-sonnet-5-5"
# aegis, the grader, and coder are built for Linux, and aegis and the
# grader run on this host.
[[ $(uname -s) == Linux ]] || die "a Linux host is required; this host is $(uname -s)"
# Grading must not run as root (see grade_isolated); check before any build
# or paid trial.
if [[ $validate_only -eq 0 && $self_check -eq 0 && $(id -u) -eq 0 ]]; then
	die "grading must not run as root; run as an unprivileged user"
fi
if [[ -z $concurrency ]]; then
	concurrency=$(($(nproc) / 2))
	((concurrency >= 1)) || concurrency=1
	((concurrency <= 8)) || concurrency=8
fi

suite_dir() {
	case "$1" in
	lifecycle) echo lifecycle ;;
	tool-guidance) echo toolguidance ;;
	tool-descriptions) echo tooldescriptions ;;
	correctness) echo correctness ;;
	terminal-bench) echo terminalbench ;;
	*) return 1 ;;
	esac
}

# suite_has_case reports whether a suite defines case id $2. Case ids are
# the top-level id: "..." fields of the cases array in EVAL.ts (check ids
# sit deeper or share a line with other fields), or ids in
# terminalbench/tasks.json.
suite_has_case() {
	local dir
	dir=$(suite_dir "$1")
	if [[ $1 == terminal-bench ]]; then
		jq -e --arg id "$2" '.tasks | any(.id == $id)' "$EVALS_DIR/$dir/tasks.json" >/dev/null
	else
		awk -v want="    id: \"$2\"," '
			/^const cases/ { in_cases = 1; next }
			in_cases && /^\];/ { in_cases = 0 }
			in_cases && $0 == want { found = 1 }
			END { exit !found }
		' "$EVALS_DIR/$dir/EVAL.ts"
	fi
}

if [[ ${#suites[@]} -eq 0 ]]; then
	if [[ ${#cases[@]} -gt 0 ]]; then
		suites=("${ALL_SUITES[@]}")
	elif [[ $self_check -eq 1 ]]; then
		suites=(terminal-bench)
	else
		suites=("${DEFAULT_SUITES[@]}")
	fi
fi
selected=()
for s in "${suites[@]}"; do
	suite_dir "$s" >/dev/null || die "unknown suite $s"
	[[ " ${selected[*]-} " == *" $s "* ]] && continue
	if [[ ${#cases[@]} -gt 0 ]]; then
		match=0
		for c in "${cases[@]}"; do
			if suite_has_case "$s" "$c"; then
				match=1
			fi
		done
		[[ $match -eq 1 ]] || continue
	fi
	selected+=("$s")
done
[[ ${#selected[@]} -gt 0 ]] || die "no suite defines the requested cases"
for c in "${cases[@]}"; do
	found=0
	for s in "${selected[@]}"; do
		if suite_has_case "$s" "$c"; then
			found=1
		fi
	done
	[[ $found -eq 1 ]] || die "no selected suite defines case $c"
done
if [[ $self_check -eq 1 && " ${selected[*]} " != *" terminal-bench "* ]]; then
	die "--self-check applies to --suite terminal-bench"
fi

for tool in docker git go jq mise; do
	command -v "$tool" >/dev/null || die "$tool is required"
done
# The grader's docker commands read no docker config, and aegis passes
# subprocess graders no DOCKER_HOST, so pin the daemon that the docker CLI
# selects here (DOCKER_HOST, DOCKER_CONTEXT, or the current context).
DOCKER_HOST=$(docker context inspect --format '{{.Endpoints.docker.Host}}') || die "cannot resolve the docker daemon; set DOCKER_HOST"
export DOCKER_HOST

mkdir -p "$CACHE"

cleanup_paths=()
# Grading containers hold nothing worth keeping, so they go whatever
# --keep says, also when run.sh is interrupted.
cleanup_containers=()
cleanup() {
	local p c
	for c in "${cleanup_containers[@]}"; do
		docker rm -f "$c" >/dev/null || echo "run.sh: docker rm -f $c failed; remove the container by hand" >&2
	done
	if [[ $keep -eq 1 ]]; then
		return
	fi
	for p in "${cleanup_paths[@]}"; do
		if [[ -d $p ]]; then
			git -C "$REPO_ROOT" worktree remove --force "$p" >/dev/null 2>&1 || rm -rf "$p"
		fi
	done
}
trap cleanup EXIT

# Commands run inside the aegis checkout must see its mise toolchain rather
# than a host GOROOT or GOBIN.
aegis_env() {
	local dir=$1
	shift
	(cd "$dir" && env -u GOROOT -u GOBIN MISE_TRUSTED_CONFIG_PATHS="$dir" MISE_YES=1 \
		MISE_TASK_RUN_AUTO_INSTALL=false MISE_EXEC_AUTO_INSTALL=false "$@")
}

# aegis_tool_path prints PATH with the aegis checkout's pinned go, node, and
# pnpm first. mise tasks do not always put them ahead of a host or system
# mise toolchain, and content-tag.sh and the sdk's package.json engines
# refuse other versions.
aegis_tool_path() {
	local paths
	paths=$(aegis_env "$1" mise bin-paths go node pnpm | paste -sd: -)
	[[ -n $paths ]] || die "mise found no go, node, or pnpm for $1"
	printf '%s:%s\n' "$paths" "$PATH"
}

bootstrap_aegis() {
	local ref=${CHATD_EVAL_AEGIS_REF:-$AEGIS_REF_DEFAULT} src tmp tool_path
	src=$AEGIS_REPO
	if [[ -n ${CHATD_EVAL_AEGIS_DIR:-} ]]; then
		# The checkout the user passed in is only read: clone its commit.
		src=$CHATD_EVAL_AEGIS_DIR
		ref=$(git -C "$src" rev-parse HEAD)
	fi
	AEGIS_DIR=$CACHE/aegis/$ref
	if [[ ! -f $AEGIS_DIR/.chatd-evals-ready ]]; then
		log "building aegis $ref"
		mkdir -p "$CACHE/aegis"
		# A per-run build directory keeps concurrent runs on a cold cache from
		# clobbering each other's half-built checkout.
		tmp=$(mktemp -d "$AEGIS_DIR.tmp.XXXXXX")
		cleanup_paths+=("$tmp")
		git clone -q "$src" "$tmp"
		git -C "$tmp" checkout -q --detach "$ref"
		{
			# Only the tools these steps use: a bare mise install would also
			# install every tool in the user's global mise config.
			aegis_env "$tmp" mise install go node pnpm
			tool_path=$(aegis_tool_path "$tmp")
			aegis_env "$tmp" env PATH="$tool_path" mise run evals-build
			aegis_env "$tmp" mise exec -- env CGO_ENABLED=0 GOOS=linux go build -o bin/aegis ./cmd/aegis
		} >&2
		touch "$tmp/.chatd-evals-ready"
		# A concurrent run that finished first may already be using its copy.
		if [[ -f $AEGIS_DIR/.chatd-evals-ready ]]; then
			rm -rf "$tmp"
		else
			rm -rf "$AEGIS_DIR"
			mv "$tmp" "$AEGIS_DIR"
		fi
	fi
	AEGIS=$AEGIS_DIR/bin/aegis
	NODE_BIN=$(aegis_env "$AEGIS_DIR" mise where node)/bin
	export PATH="$NODE_BIN:$PATH"
}

runner_image() {
	local arch go_path
	arch=$(docker version --format '{{.Server.Arch}}')
	go_path=$(aegis_tool_path "$AEGIS_DIR")
	RUNNER_TAG=$(aegis_env "$AEGIS_DIR" env PATH="$go_path" bash images/runner/content-tag.sh "$arch")
	RUNNER_IMAGE=aegis-runner:$RUNNER_TAG
	if ! docker image inspect "$RUNNER_IMAGE" >/dev/null 2>&1; then
		log "building runner image $RUNNER_IMAGE"
		aegis_env "$AEGIS_DIR" env PATH="$go_path" mise run image >&2
	fi
}

build_grader() {
	GRADER=$CACHE/bin/chatd-eval-grader
	mkdir -p "$CACHE/bin"
	log "building chatd-eval-grader"
	# Per-run temporary names let concurrent runs share the cache; mv
	# replaces the binary atomically.
	(cd "$REPO_ROOT" && CGO_ENABLED=0 GOOS=linux go build -o "$GRADER.new.$$" ./coderd/x/chatd/evals/grader)
	mv "$GRADER.new.$$" "$GRADER"
}

# build_subject sets SUBJECT_IMAGE, SUBJECT_VERSION, and SUBJECT_CUSTOM for
# the coder build named by $1 ("" for this checkout).
build_subject() {
	local want=$1 src sha id bin dirty_hash ctx tag version
	if [[ $want == release ]]; then
		SUBJECT_IMAGE=$RUNNER_IMAGE
		SUBJECT_VERSION=2.37.0
		SUBJECT_CUSTOM=0
		return
	fi
	if [[ -z $want ]]; then
		src=$REPO_ROOT
		sha=$(git -C "$src" rev-parse HEAD)
		id=$sha
		# Dirty trees get a version of their own so a declared version never
		# names two different builds. The evals directory is not part of the
		# coder binary, so editing suites does not force a rebuild.
		if [[ -n $(git -C "$src" status --porcelain -- . ":(exclude)$EVALS_PATHSPEC") ]]; then
			dirty_hash=$(
				cd "$src"
				{
					git diff HEAD --binary -- . ":(exclude)$EVALS_PATHSPEC"
					# git records an untracked symlink as its target path,
					# which may be a directory or missing.
					git ls-files --others --exclude-standard -z -- . ":(exclude)$EVALS_PATHSPEC" |
						while IFS= read -r -d '' f; do
							if [[ -L $f ]]; then
								printf 'symlink %s -> %s\n' "$f" "$(readlink -- "$f")"
							else
								sha256sum -- "$f"
							fi
						done
				} | sha256sum | cut -c1-8
			)
			id=$id.dirty.$dirty_hash
		fi
	else
		sha=$(git -C "$REPO_ROOT" rev-parse --verify "$want^{commit}") || die "unknown ref $want"
		id=$sha
		# The worktree is created only when the binary needs a build.
		src=""
	fi
	# Stamp the version with the full commit: go build does not embed VCS
	# data from every checkout (a linked worktree, for one), an unstamped
	# build reports a bare 0.0.0-devel that two different commits would
	# share, and two commits can share a short prefix. The version names the
	# cached binary and the image tag.
	version=0.0.0-devel+$id
	bin=$CACHE/coder/coder-$id
	mkdir -p "$CACHE/coder"
	if [[ ! -x $bin || $id == *.dirty.* || $("$bin" version --output json | jq -r '.version') != "v$version" ]]; then
		log "building coder ${id:0:7}${id:40}"
		if [[ -z $src ]]; then
			mkdir -p "$CACHE/worktrees"
			src=$(mktemp -d "$CACHE/worktrees/$sha.XXXXXX")
			cleanup_paths+=("$src")
			git -C "$REPO_ROOT" worktree prune
			git -C "$REPO_ROOT" worktree add -q --detach "$src" "$sha"
		fi
		(cd "$src" && CGO_ENABLED=0 go build -ldflags "-X 'github.com/coder/coder/v2/buildinfo.tag=$version'" -o "$bin.new.$$" ./enterprise/cmd/coder)
		mv "$bin.new.$$" "$bin"
	fi
	SUBJECT_VERSION=$("$bin" version --output json | jq -r '.version | ltrimstr("v")')
	[[ $SUBJECT_VERSION == "$version" ]] || die "$bin reports version $SUBJECT_VERSION, want $version"
	tag=$(printf '%s' "$SUBJECT_VERSION" | tr -c 'A-Za-z0-9_.-' '-' | cut -c1-100)-$RUNNER_TAG
	SUBJECT_IMAGE=chatd-evals-runner:$tag
	SUBJECT_CUSTOM=1
	if ! docker image inspect "$SUBJECT_IMAGE" >/dev/null 2>&1; then
		log "building $SUBJECT_IMAGE"
		ctx=$(mktemp -d)
		cp "$bin" "$ctx/coder"
		printf 'FROM %s\nCOPY --chmod=0755 coder /usr/local/bin/coder\n' "$RUNNER_IMAGE" >"$ctx/Dockerfile"
		docker build -q -t "$SUBJECT_IMAGE" "$ctx" >/dev/null
		rm -rf "$ctx"
	fi
}

# Credentials go to aegis through its upstream variables; values are never
# printed or written.
map_credentials() {
	case "$model" in
	anthropic:*)
		[[ -n ${ANTHROPIC_BASE_URL:-} ]] && export AEGIS_UPSTREAM_ANTHROPIC_BASE_URL=$ANTHROPIC_BASE_URL
		if [[ -n ${ANTHROPIC_AUTH_TOKEN:-} ]]; then
			export AEGIS_UPSTREAM_AUTH_TOKEN=$ANTHROPIC_AUTH_TOKEN
		elif [[ -n ${ANTHROPIC_API_KEY:-} ]]; then
			export AEGIS_UPSTREAM_ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY
		elif [[ -z ${AEGIS_UPSTREAM_AUTH_TOKEN:-}${AEGIS_UPSTREAM_ANTHROPIC_API_KEY:-} ]]; then
			die "set ANTHROPIC_AUTH_TOKEN or ANTHROPIC_API_KEY"
		fi
		;;
	openai:*)
		[[ -n ${OPENAI_BASE_URL:-} ]] && export AEGIS_UPSTREAM_OPENAI_BASE_URL=$OPENAI_BASE_URL
		if [[ -n ${OPENAI_API_KEY:-} ]]; then
			export AEGIS_UPSTREAM_OPENAI_AUTH_TOKEN=$OPENAI_API_KEY
		elif [[ -z ${AEGIS_UPSTREAM_OPENAI_AUTH_TOKEN:-} ]]; then
			die "set OPENAI_API_KEY"
		fi
		;;
	*)
		die "unsupported model provider in $model"
		;;
	esac
	return 0
}

terminal_bench_import() {
	local repo=$CACHE/terminal-bench/repo-${TB_COMMIT:0:7} args=() c tmp
	if [[ ! -d $repo/tasks ]]; then
		log "fetching Terminal-Bench ${TB_COMMIT:0:7}"
		mkdir -p "$CACHE/terminal-bench"
		tmp=$(mktemp -d "$repo.tmp.XXXXXX")
		cleanup_paths+=("$tmp")
		git init -q "$tmp"
		git -C "$tmp" fetch -q --depth 1 "$TB_REPO" "$TB_COMMIT"
		git -C "$tmp" checkout -q --detach FETCH_HEAD
		if [[ -d $repo/tasks ]]; then
			rm -rf "$tmp"
		else
			rm -rf "$repo"
			mv "$tmp" "$repo"
		fi
	fi
	TB_DIR=$CACHE/terminal-bench/import-${TB_COMMIT:0:7}
	for c in "${cases[@]}"; do
		if suite_has_case terminal-bench "$c"; then
			args+=(--task "$c")
		fi
	done
	log "importing Terminal-Bench tasks"
	"$GRADER" tb-import --tasks "$EVALS_DIR/terminalbench/tasks.json" --repo "$repo" --out "$TB_DIR" "${args[@]}" >&2
	export CHATD_EVAL_TB_DIR=$TB_DIR
}

# validate_suite compiles one suite and prints its bundle digest.
validate_suite() {
	local file=$1 line
	line=$("$AEGIS" validate "$file" 2>&1 | grep -v -e MODULE_TYPELESS -e 'Reparsing as ES' -e 'eliminate this warning' -e 'trace-warnings') || true
	if [[ $line != "eval bundle valid:"* ]]; then
		echo "$line" >&2
		return 1
	fi
	echo "$line" >&2
	sed -n 's/.*digest=sha256:\([0-9a-f]*\).*/\1/p' <<<"$line"
}

# grade_isolated runs aegis collect inside a locked-down copy of the subject
# image (aegis docs/how-to/benchmark-agents.md, isolated grading recipe):
# no network, no credentials, read-only store, and outputs streamed off a
# size-capped tmpfs. Hidden tests and suite setup execute agent-modified
# code, so they never run on this host.
grade_isolated() {
	local store=$1 digest=$2 dest=$3 image=$4 name rc=0 kept c
	name=chatd-evals-grade-$$-$RANDOM
	docker run -d --name "$name" --pull=never --network none \
		--user "$(id -u):$(id -g)" --read-only \
		--tmpfs /tmp:rw,exec,size=2g,mode=1777 --tmpfs /work:rw,size=256m,mode=1777 \
		--memory 4g --memory-swap 4g --pids-limit 1024 --cpus 2 \
		--security-opt no-new-privileges --cap-drop ALL \
		-v "$store:/store:ro" -v "$AEGIS:/usr/local/bin/aegis:ro" -v "$GRADER:$GRADER:ro" \
		-e PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
		--entrypoint sleep "$image" infinity >/dev/null || return 1
	cleanup_containers+=("$name")
	docker exec -w /work "$name" aegis collect --store /store \
		--bundle "/store/bundle-assets/$digest/eval_bundle.json" --out derived --regrade \
		--allow-unsafe-host-graders --allow-unsafe-host-subprocess-graders \
		--allow-subprocess-binary "$GRADER" >&2 || rc=$?
	if ! docker exec "$name" find /work -mindepth 1 -exec chmod a+rX {} +; then
		rc=1
	fi
	mkdir "$dest" && docker exec -u 0 "$name" sh -euc '
		d=$(mktemp -d)
		cp -a --no-preserve=ownership /work "$d/snap"
		cd "$d/snap"
		[ -z "$(find . -mindepth 1 ! -type f ! -type d | head -n 1)" ]
		find . -mindepth 1 -print0 > "$d/all"
		head -zn 10001 "$d/all" > "$d/list"
		[ "$(tr -cd "\0" < "$d/list" | wc -c)" -le 10000 ]
		tar -S --null --verbatim-files-from --no-recursion -cf - -T "$d/list"' >"$dest.tar" &&
		tar -S -xf "$dest.tar" -C "$dest" || rc=1
	rm -f "$dest.tar"
	if [[ -n $(find "$dest" -mindepth 1 ! -type f ! -type d | head -n 1) ]]; then
		echo "refusing grading output with non-regular entries" >&2
		rm -rf "$dest"
		rc=1
	fi
	if docker rm -f "$name" >/dev/null; then
		kept=()
		for c in "${cleanup_containers[@]}"; do
			[[ $c == "$name" ]] || kept+=("$c")
		done
		cleanup_containers=("${kept[@]}")
	else
		echo "run.sh: docker rm -f $name failed; retrying at exit" >&2
	fi
	return "$rc"
}

# run_suite runs, grades, and summarizes one suite on the current subject.
# Callers run it as `run_suite ... || status=1`, which turns off errexit
# inside it, so every step's status goes into rc explicitly.
run_suite() {
	local suite=$1 label=$2 dir file digest dest store rc=0 grade_rc=0 arms run_args grading
	dir=$(suite_dir "$suite")
	file=$EVALS_DIR/$dir/EVAL.ts
	dest=$out/$label/$suite
	store=$dest/store
	mkdir -p "$dest"
	export CHATD_EVAL_IMAGE=$SUBJECT_IMAGE CHATD_EVAL_CODER_VERSION=$SUBJECT_VERSION CHATD_EVAL_CUSTOM_CODER=$SUBJECT_CUSTOM
	log "$label/$suite: validating ($SUBJECT_VERSION)"
	digest=$(validate_suite "$file") || die "$suite does not validate"
	run_args=(run "$file" --provider docker --store "$store" --concurrency "$concurrency" --confirm-budget)
	[[ -n $max_usd ]] && run_args+=(--max-usd "$max_usd")
	log "$label/$suite: running trials"
	(cd "$dest" && "$AEGIS" "${run_args[@]}") 2>&1 | tee "$dest/run.log" >&2 || rc=$?
	[[ $rc -eq 0 ]] || log "$label/$suite: aegis run exited $rc; grading the trials that landed"
	[[ -d $store/bundles ]] || die "$label/$suite: no trials landed (see $dest/run.log)"
	log "$label/$suite: grading"
	# Grade next to the earlier grading and replace it only when grading
	# succeeds, so a resume that cannot be graded (another bundle digest,
	# for one) keeps the earlier verdicts.
	grading=$dest/grading.new.$$
	rm -rf "$grading"
	if [[ $suite == terminal-bench ]]; then
		mkdir -p "$grading"
		"$AEGIS" collect --store "$store" --bundle "$store/bundle-assets/$digest/eval_bundle.json" \
			--out "$grading/derived" --regrade \
			--allow-unsafe-host-subprocess-graders --allow-subprocess-binary "$GRADER" >&2 || grade_rc=$?
	else
		grade_isolated "$store" "$digest" "$grading" "$SUBJECT_IMAGE" || grade_rc=$?
	fi
	if [[ $grade_rc -eq 0 || ! -d $dest/grading ]]; then
		rm -rf "$dest/grading"
		mv "$grading" "$dest/grading" || rc=1
		# A failed attempt that this grading supersedes would only mislead.
		[[ $grade_rc -ne 0 ]] || rm -rf "$dest/grading.failed"
	else
		rm -rf "$dest/grading.failed"
		if [[ -n $(find "$grading" -type f -print -quit 2>/dev/null) ]]; then
			mv "$grading" "$dest/grading.failed" || rc=1
			log "$label/$suite: grading failed; keeping the earlier grading, this attempt is in $dest/grading.failed"
		else
			rm -rf "$grading"
			log "$label/$suite: grading failed before collection wrote any output; keeping the earlier grading"
		fi
	fi
	[[ $grade_rc -eq 0 ]] || rc=$grade_rc
	arms=$(jq -r '.experiment.variants | length' "$store/bundle-assets/$digest/eval_bundle.json") || return 1
	if [[ -n $prompt_file && $arms -lt 2 ]]; then
		log "$label/$suite: --prompt-file is set but the bundle declares $arms arm(s); no contrast report"
		rc=1
	fi
	if [[ $arms -ge 2 ]]; then
		"$AEGIS" trials --store "$store" --derived "$dest/grading/derived" \
			--bundle "$store/bundle-assets/$digest/eval_bundle.json" --out "$dest/trials.json" >&2 &&
			"$AEGIS" report --in "$dest/trials.json" --bundle "$store/bundle-assets/$digest/eval_bundle.json" \
				--out-dir "$dest/report" --store "$dest/report-history" >&2 || rc=$?
	fi
	"$GRADER" summarize --store "$store" --derived "$dest/grading/derived" --label "$label $SUBJECT_VERSION" \
		--json "$dest/summary.json" | tee "$dest/summary.txt" || rc=$?
	return "$rc"
}

compare_suite() {
	local suite=$1
	"$GRADER" summarize --store "$out/candidate/$suite/store" --derived "$out/candidate/$suite/grading/derived" \
		--label "candidate $CANDIDATE_VERSION" --baseline-store "$out/baseline/$suite/store" \
		--baseline-derived "$out/baseline/$suite/grading/derived" --baseline-label "baseline $BASELINE_VERSION" \
		--json "$out/comparison-$suite.json" | tee "$out/comparison-$suite.txt"
}

# validate_selected compiles and validates every selected suite with the
# current CHATD_EVAL_* environment.
validate_selected() {
	local s
	for s in "${selected[@]}"; do
		log "validating $s"
		validate_suite "$EVALS_DIR/$(suite_dir "$s")/EVAL.ts" >/dev/null || die "$s does not validate"
	done
}

bootstrap_aegis
# The inputs a run compiles into its suites. --validate-only checks the same
# configuration, so a pin or budget that cannot run fails there too.
export CHATD_EVAL_MODEL=$model CHATD_EVAL_REPEATS=$repeats
export CHATD_EVAL_MAX_USD=$max_usd CHATD_EVAL_PROMPT_FILE=$prompt_file CHATD_EVAL_PROMPT_MODE=$prompt_mode
CHATD_EVAL_CASES=$(
	IFS=,
	echo "${cases[*]-}"
)
export CHATD_EVAL_CASES
if [[ $validate_only -eq 1 ]]; then
	validate_selected
	exit 0
fi
runner_image
build_grader
export CHATD_EVAL_GRADER=$GRADER

if [[ " ${selected[*]} " == *" terminal-bench "* ]]; then
	terminal_bench_import
	if [[ $self_check -eq 1 ]]; then
		args=()
		for c in "${cases[@]}"; do
			args+=(--task "$c")
		done
		"$GRADER" tb-selfcheck --tb-dir "$TB_DIR" "${args[@]}"
		exit 0
	fi
fi
# A configuration that cannot validate fails here, before the results
# directory and the coder build.
validate_selected

# The default output lives in the cache: under a checkout it would be
# untracked content that marks the next default build dirty.
if [[ -z $out ]]; then
	out=$CACHE/runs/$(date -u +%Y%m%dT%H%M%SZ)-$$
fi
mkdir -p "$out"
out=$(cd "$out" && pwd)
log "writing results to $out"

map_credentials

status=0
build_subject "$ref"
log "candidate build: coder $SUBJECT_VERSION ($SUBJECT_IMAGE)"
CANDIDATE_VERSION=$SUBJECT_VERSION
for s in "${selected[@]}"; do
	run_suite "$s" candidate || status=1
done
if [[ -n $baseline_ref ]]; then
	build_subject "$baseline_ref"
	log "baseline build: coder $SUBJECT_VERSION ($SUBJECT_IMAGE)"
	BASELINE_VERSION=$SUBJECT_VERSION
	for s in "${selected[@]}"; do
		run_suite "$s" baseline || status=1
	done
	for s in "${selected[@]}"; do
		compare_suite "$s" || status=1
	done
fi
log "results in $out"
exit "$status"
