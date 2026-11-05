#!/usr/bin/env bash
# Model-checks ProvisionerJobs.tla with TLC and compares each config with its
# expected result. Prints one line per config and a count line. Exits 0 only
# when every config matches.
#
# --sync records the current hash of every covered file in coverage.txt. Run
# it only after you confirm that the model describes those files.
#
# Env: OUT (keep TLC output there, default a temp dir), JAVA (java binary).
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(git -C "$here" rev-parse --show-toplevel)"
coverage="$here/coverage.txt"

if [[ "${1:-}" == "--sync" ]]; then
	tmp="$(mktemp)"
	while IFS= read -r line; do
		if [[ -z "$line" || "$line" == \#* ]]; then
			echo "$line"
		else
			path="${line%% *}"
			echo "$path $(git -C "$repo" hash-object "$path")"
		fi
	done <"$coverage" >"$tmp"
	mv "$tmp" "$coverage"
	echo "recorded $(grep -cvE '^(#|$)' "$coverage") covered files in coverage.txt"
	exit 0
fi

tla_version="1.7.4"
tla_sha256="936a262061c914694dfd669a543be24573c45d5aa0ff20a8b96b23d01e050e88"
cache="$repo/build/verify-provisioner-jobs"
jar="$cache/tla2tools-$tla_version.jar"

# config|expected: PASS, or the name of the property TLC must report broken.
checks=(
	"Safety|PASS"
	"NegAcquireNoLock|OneRunnerPerJob"
	"NegCancelIgnoresWorker|CancelNeverClosesRunningJob"
	"NegPrebuildCancelReadCommitted|CancelNeverClosesRunningJob"
	"NegReaperNoStaleRecheck|ReaperSparesLiveJob"
	"NegReaperNoCompletedRecheck|OutcomeIsFinal"
	"NegLateFailDropsState|LatestStateTracksApply"
	"NegLateWriteRejected|LatestStateTracksApply"
	"NotPromisedOutcomeIsFinal|OutcomeIsFinal"
	"NotPromisedNextBuildSeesApply|NextBuildSeesApply"
)

if [[ ! -f "$jar" ]] || ! echo "$tla_sha256  $jar" | sha256sum --check --status; then
	mkdir -p "$cache"
	curl -fsSL -o "$jar.tmp" "https://github.com/tlaplus/tlaplus/releases/download/v$tla_version/tla2tools.jar"
	echo "$tla_sha256  $jar.tmp" | sha256sum --check --status || {
		echo "tla2tools.jar checksum mismatch" >&2
		exit 2
	}
	mv "$jar.tmp" "$jar"
fi

java_cmd=()
if [[ -n "${JAVA:-}" ]]; then
	java_cmd=("$JAVA")
elif command -v java >/dev/null 2>&1; then
	java_cmd=(java)
elif command -v mise >/dev/null 2>&1; then
	java_cmd=(mise exec java@temurin-25 -- java)
else
	echo "need java 11 or newer on PATH, JAVA set, or mise" >&2
	exit 2
fi

out="${OUT:-$(mktemp -d)}"
mkdir -p "$out"
meta="$(mktemp -d)"
trap 'rm -rf "$meta"' EXIT

ok=0
bad=0
for check in "${checks[@]}"; do
	cfg="${check%%|*}"
	want="${check##*|}"
	log="$out/$cfg.out"
	set +e
	(cd "$here" && "${java_cmd[@]}" -XX:+UseParallelGC -cp "$jar" tlc2.TLC \
		-workers 1 -metadir "$meta/$cfg" -config "$cfg.cfg" ProvisionerJobs.tla) >"$log" 2>&1
	set -e
	if grep -q "^Model checking completed. No error has been found." "$log"; then
		got="PASS"
	else
		got="$(sed -nE 's/^Error: (Invariant|Action property) ([A-Za-z0-9_]+) is violated\.$/\2/p' "$log" | head -n 1)"
		if [[ -z "$got" ]]; then
			got="ERROR($(grep -m 1 '^Error:' "$log" || echo "no TLC verdict"))"
		fi
	fi
	states="$(sed -nE 's/^([0-9,]+) states generated, ([0-9,]+) distinct states found.*/\2/p' "$log" | tail -n 1)"
	if [[ "$got" == "$want" ]]; then
		ok=$((ok + 1))
		echo "ok   $cfg: $got (${states:-?} distinct states)"
	else
		bad=$((bad + 1))
		echo "FAIL $cfg: want $want, got $got (log $log)"
	fi
done

changed=()
while IFS=' ' read -r path recorded; do
	[[ -z "$path" || "$path" == \#* ]] && continue
	if [[ "$(git -C "$repo" hash-object "$path" 2>/dev/null || echo missing)" != "$recorded" ]]; then
		changed+=("$path")
	fi
done <"$coverage"
if [[ "${#changed[@]}" -gt 0 ]]; then
	echo "covered files changed after the last sync:"
	for path in "${changed[@]}"; do
		echo "  $path"
	done
	echo "WARNING: the results above describe the spec, not these changes."
	echo "  Update ProvisionerJobs.tla to match them first (SKILL.md, \"When your change is intended\")."
else
	echo "covered files unchanged since the last sync"
fi

echo "TLC output: $out"
echo "checks: $ok ok, $bad failed, ${#checks[@]} total"
[[ "$bad" -eq 0 ]]
