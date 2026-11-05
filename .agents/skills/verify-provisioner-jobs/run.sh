#!/usr/bin/env bash
# Model-checks ProvisionerJobs.tla with TLC and compares each config with its
# expected result. Prints one line per config and a count line. Exits 0 only
# when every config matches.
#
# Env: OUT (keep TLC output there, default a temp dir), JAVA (java binary).
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(git -C "$here" rev-parse --show-toplevel)"

tla_version="1.7.4"
tla_sha256="936a262061c914694dfd669a543be24573c45d5aa0ff20a8b96b23d01e050e88"
cache="$repo/build/verify-provisioner-jobs"
jar="$cache/tla2tools-$tla_version.jar"

# config|expected: PASS, or the name of the property TLC must report broken.
checks=(
	"Safety|PASS"
	"Fixed|PASS"
	"F1Outcome|OutcomeIsFinal"
	"F1NextBuild|NextBuildStartsFromFinalState"
	"NegAcquireNoLock|OneRunnerPerJob"
	"NegCancelIgnoresWorker|CancelNeverClosesRunningJob"
	"NegPrebuildCancelReadCommitted|CancelNeverClosesRunningJob"
	"NegReaperNoStaleRecheck|ReaperSparesLiveJob"
	"NegReaperNoCompletedRecheck|OutcomeIsFinal"
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

sync="$(sed -n 's/^sync //p' "$here/coverage.txt")"
mapfile -t covered < <(grep -vE '^(#|sync |$)' "$here/coverage.txt")
if git -C "$repo" cat-file -e "$sync^{commit}" 2>/dev/null; then
	changed="$(git -C "$repo" diff --name-only "$sync" -- "${covered[@]}")"
	if [[ -n "$changed" ]]; then
		echo "covered code changed after sync commit ${sync:0:10}:"
		while IFS= read -r path; do
			echo "  $path"
		done <<<"$changed"
		echo "WARNING: the results above describe the spec, not these changes."
		echo "  Update ProvisionerJobs.tla to match them first (SKILL.md, \"When your change is intended\")."
	else
		echo "covered code unchanged after sync commit ${sync:0:10}"
	fi
else
	echo "sync commit ${sync:0:10} not in this clone: fetch more history to compare"
fi

echo "TLC output: $out"
echo "checks: $ok ok, $bad failed, ${#checks[@]} total"
[[ "$bad" -eq 0 ]]
