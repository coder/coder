#!/usr/bin/env bash
# context-on-clone.sh
#
# Runs after a repository clone so generated instructions and skills stay
# grouped under the repository that owns them.
#
# Environment variables:
#   CONTEXT_SERVICE_URL    - External context service endpoint (optional).
#   CONTEXT_SERVICE_TOKEN  - Bearer token for the service (optional).
#   GENERATED_CONTEXT_ROOT - Root directory for generated repository context.
#   INSTRUCTIONS_DIR       - Legacy fallback for GENERATED_CONTEXT_ROOT.
#   SKILLS_DIR             - Legacy fallback when no root is provided.

set -euo pipefail

log() {
	echo "[context-on-clone] $*"
}

expand_path() {
	echo "${1/#\~/$HOME}"
}

write_file() {
	local dest="$1"
	local content="$2"

	mkdir -p "$(dirname "$dest")"
	printf '%s\n' "$content" >"$dest"
	log "Wrote $(wc -c <"$dest") bytes to $dest"
}

resolve_context_root() {
	if [ -n "${GENERATED_CONTEXT_ROOT:-}" ]; then
		expand_path "$GENERATED_CONTEXT_ROOT"
		return
	fi

	if [ -n "${INSTRUCTIONS_DIR:-}" ]; then
		expand_path "$INSTRUCTIONS_DIR"
		return
	fi

	if [ -n "${SKILLS_DIR:-}" ]; then
		expand_path "$SKILLS_DIR"
		return
	fi

	expand_path "$HOME/.coder/generated-context"
}

add_chat_context() {
	if ! command -v coder >/dev/null 2>&1; then
		log "Skipping chat context injection because the coder CLI is unavailable."
		return 0
	fi

	local output
	if output="$(coder chat context add --dir "$REPO_CONTEXT_DIR" 2>&1)"; then
		log "Injected chat context from $REPO_CONTEXT_DIR"
		if [ -n "$output" ]; then
			log "$output"
		fi
	else
		log "Chat context injection skipped: $output"
	fi
}

REPO_DIR="${1:-${CODER_GIT_CLONE_DIR:-$(pwd)}}"
REPO_DIR="$(expand_path "$REPO_DIR")"
REPO_NAME="$(basename "$REPO_DIR")"
CONTEXT_ROOT="$(resolve_context_root)"
REPO_CONTEXT_DIR="$CONTEXT_ROOT/$REPO_NAME"
AGENTS_FILE="$REPO_CONTEXT_DIR/AGENTS.md"
REPO_SKILLS_DIR="$REPO_CONTEXT_DIR/.agents/skills"

log "Repository: $REPO_NAME ($REPO_DIR)"
log "Context root: $CONTEXT_ROOT"
log "Repository context dir: $REPO_CONTEXT_DIR"

RESPONSE=""

if [ -n "${CONTEXT_SERVICE_URL:-}" ]; then
	log "Calling context service at $CONTEXT_SERVICE_URL"

	CURL_ARGS=(
		--silent
		--show-error
		--fail
		--max-time 30
		--header "Content-Type: application/json"
	)

	if [ -n "${CONTEXT_SERVICE_TOKEN:-}" ]; then
		CURL_ARGS+=(--header "Authorization: Bearer $CONTEXT_SERVICE_TOKEN")
	fi

	PAYLOAD="$(
		cat <<JSON
{
  "repo_name": "$REPO_NAME",
  "repo_dir": "$REPO_DIR",
  "workspace_owner": "${CODER_WORKSPACE_OWNER_NAME:-unknown}",
  "workspace_name": "${CODER_WORKSPACE_NAME:-unknown}"
}
JSON
	)"

	if RESPONSE="$(curl "${CURL_ARGS[@]}" -d "$PAYLOAD" "$CONTEXT_SERVICE_URL")"; then
		log "Context service responded successfully"
	else
		log "WARNING: Context service call failed. Using fallback."
		RESPONSE=""
	fi
fi

if [ -z "$RESPONSE" ]; then
	log "Using built-in mock context for $REPO_NAME"
	RESPONSE="$(
		cat <<MOCK
{
  "instructions": "# $REPO_NAME\n\nThis is generated context for the $REPO_NAME repository.\n\n## Repository Layout\n\nExplore the cloned repository at \`$REPO_DIR\` to understand the project structure.\n\n## Conventions\n\n- Follow the existing code style in the repository.\n- Write tests for new functionality.\n- Keep commits focused and well-described.",
  "skills": [
    {
      "name": "repo-overview",
      "description": "Provide an overview of this repository.",
      "body": "---\nname: repo-overview\ndescription: Provide an overview of the $REPO_NAME repository.\n---\n\n# Repo Overview\n\nWhen asked about the repository, explore the directory structure at \`$REPO_DIR\` and summarize the project layout, key technologies, and development patterns."
    }
  ]
}
MOCK
	)"
fi

INSTRUCTIONS="$(echo "$RESPONSE" | python3 -c '
import sys, json

data = json.load(sys.stdin)
print(data.get("instructions", ""))
' 2>/dev/null || echo "")"

if [ -n "$INSTRUCTIONS" ]; then
	write_file "$AGENTS_FILE" "$INSTRUCTIONS"
else
	log "WARNING: No instructions in response. Skipping AGENTS.md."
fi

if [ -d "$REPO_SKILLS_DIR" ]; then
	rm -rf "$REPO_SKILLS_DIR"
	log "Cleared $REPO_SKILLS_DIR"
fi

SKILL_COUNT="$(echo "$RESPONSE" | python3 -c '
import sys, json

data = json.load(sys.stdin)
print(len(data.get("skills", [])))
' 2>/dev/null || echo "0")"

if [ "$SKILL_COUNT" -gt 0 ]; then
	log "Writing $SKILL_COUNT skill(s)"

	for i in $(seq 0 $((SKILL_COUNT - 1))); do
		SKILL_NAME="$(echo "$RESPONSE" | python3 -c '
import sys, json

index = int(sys.argv[1])
data = json.load(sys.stdin)
print(data["skills"][index]["name"])
' "$i")"
		SKILL_BODY="$(echo "$RESPONSE" | python3 -c '
import sys, json

index = int(sys.argv[1])
data = json.load(sys.stdin)
print(data["skills"][index]["body"])
' "$i")"
		write_file "$REPO_SKILLS_DIR/$SKILL_NAME/SKILL.md" "$SKILL_BODY"
	done
else
	log "No skills in response. Skipping skill generation."
fi

add_chat_context
log "Context discovery complete"
