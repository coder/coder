#!/usr/bin/env bash
set -euo pipefail
# shellcheck source=scripts/lib.sh
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cdroot

usage() {
	cat <<'USAGE'
Usage: scripts/audit-agent-readiness.sh [--help]

Print a report-first audit of agent harness readiness. Warnings identify
aspirational checks and do not fail the script. Missing required harness docs
fail the script. Run manually with:

  bash scripts/audit-agent-readiness.sh
USAGE
}

if [[ "${1:-}" == "--help" ]]; then
	usage
	exit 0
fi

ok_count=0
warn_count=0
fail_count=0

ok() {
	printf '[ok] %s\n' "$1"
	((ok_count++)) || true
}

warn() {
	printf '[warn] %s\n' "$1"
	((warn_count++)) || true
}

fail() {
	printf '[fail] %s\n' "$1"
	((fail_count++)) || true
}

contains() {
	local file="$1"
	local pattern="$2"
	grep -qiE "$pattern" "$file"
}

echo "Agent harness readiness audit"
echo
echo "Required harness docs"

for doc in \
	".claude/docs/OBSERVABILITY.md" \
	".claude/docs/DEV_ISOLATION.md" \
	".claude/docs/AGENT_FAILURES.md"; do
	if [[ -f "$doc" ]]; then
		ok "$doc exists."
	else
		fail "$doc is missing."
	fi
done

if [[ -L ".agents/docs" ]]; then
	agents_docs_target="$(readlink ".agents/docs")"
	if [[ "$agents_docs_target" == "../.claude/docs" ]]; then
		ok ".agents/docs points to .claude/docs."
	else
		fail ".agents/docs points to $agents_docs_target, expected ../.claude/docs."
	fi
else
	fail ".agents/docs compatibility symlink is missing."
fi

echo
echo "Navigation and report-first checks"

if contains AGENTS.md '^##[[:space:]].*(Agent navigation|Where to look)' ||
	{ grep -qF ".claude/docs/OBSERVABILITY.md" AGENTS.md &&
		grep -qF ".claude/docs/DEV_ISOLATION.md" AGENTS.md &&
		grep -qF ".claude/docs/AGENT_FAILURES.md" AGENTS.md; }; then
	ok "Root AGENTS.md appears to include agent navigation."
else
	warn "Root AGENTS.md may be missing agent navigation."
fi

if contains site/e2e/playwright.config.ts 'screenshot' &&
	contains site/e2e/playwright.config.ts 'video' &&
	contains site/e2e/playwright.config.ts 'trace' &&
	contains site/e2e/playwright.config.ts 'failure'; then
	ok "Playwright failure artifact settings appear configured."
else
	warn "Playwright failure artifact settings were not all detected."
fi

if grep -qi "playwright" .github/workflows/ci.yaml &&
	grep -q "upload-artifact" .github/workflows/ci.yaml &&
	grep -qF "failure()" .github/workflows/ci.yaml; then
	ok "E2E CI failure artifact upload appears configured."
else
	warn "E2E CI failure artifact upload was not detected."
fi

if contains .claude/docs/OBSERVABILITY.md 'Prometheus' &&
	contains .claude/docs/OBSERVABILITY.md 'log'; then
	ok "Observability doc mentions logs and Prometheus."
else
	warn "Observability doc may be missing logs or Prometheus coverage."
fi

if contains .claude/docs/DEV_ISOLATION.md 'port' &&
	contains .claude/docs/DEV_ISOLATION.md 'CODER_DEV|override'; then
	ok "Development isolation doc mentions ports and overrides."
else
	warn "Development isolation doc may be missing ports or override coverage."
fi

if grep -q 'lint/architecture' Makefile; then
	ok "Architecture lint target exists."
else
	warn "Architecture lint target is not present yet."
fi

echo
printf 'Summary: %d ok, %d warn, %d fail.\n' "$ok_count" "$warn_count" "$fail_count"

if ((fail_count > 0)); then
	exit 1
fi
