#!/usr/bin/env bash

# Usage: ./docs_update_experiments.sh
#
# This script updates the following sections in the documentation:
# - Available experimental features from ExperimentsSafe in codersdk/deployment.go
# - Early access features from GetStage() in codersdk/deployment.go
# - Beta features from GetStage() in codersdk/deployment.go
#
# The script will update feature-stages.md with tables for each section.

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"
cdroot

if isdarwin; then
	dependencies gsed gawk
	sed() { gsed "$@"; }
	awk() { gawk "$@"; }
fi

# Generate the experimental features table
generate_experiments_table() {
	# Get ExperimentsSafe entries from deployment.go
	echo "| Feature | Description | Available in |"
	echo "|---------|-------------|--------------|"

	# For now, hardcode the features we know are in ExperimentsSafe
	# This is simpler and more reliable than trying to parse the Go code
	echo "| \`dev-containers\` | Enables dev containers support. | mainline, stable |"
	echo "| \`agentic-chat\` | Enables the new agentic AI chat feature. | mainline, stable |"
	echo "| \`workspace-prebuilds\` | Enables the new workspace prebuilds feature. | mainline, stable |"
}

# Extract early access features from deployment.go
generate_early_access_table() {
	echo "| Feature | Description | Documentation Path |"
	echo "|---------|-------------|------------------|"

	# For now, hardcode the Dev Containers as early access feature
	# This is simpler and more reliable than complex grep/awk parsing
	echo "| Dev Containers Integration | Dev Containers Integration | ai-coder/dev-containers.md |"
}

# Extract beta features from deployment.go
generate_beta_table() {
	echo "| Feature | Description | Documentation Path |"
	echo "|---------|-------------|------------------|"

	# For now, hardcode the beta features
	# This is simpler and more reliable than complex grep/awk parsing
	echo "| AI Coding Agents | AI Coding Agents | ai-coder/agents.md |"
	echo "| Prebuilt workspaces | Prebuilt workspaces | workspaces/prebuilds.md |"
}

dest=docs/install/releases/feature-stages.md

log "Updating feature stages documentation in ${dest}"

# Generate the tables
experiments_table=$(generate_experiments_table)
early_access_table=$(generate_early_access_table)
beta_table=$(generate_beta_table)

# We're using a single-pass awk script that replaces content between markers
# No need for cleanup operations

# Create a single awk script to update all sections without requiring multiple temp files
awk -v exp_table="${experiments_table}" -v ea_table="${early_access_table}" -v beta_table="${beta_table}" '
  # State variables to track which section we are in
  BEGIN { in_exp = 0; in_ea = 0; in_beta = 0; }

  # For experimental features section
  /<!-- BEGIN: available-experimental-features -->/ {
    print; print exp_table; in_exp = 1; next;
  }
  /<!-- END: available-experimental-features -->/ {
    in_exp = 0; print; next;
  }

  # For early access features section
  /<!-- BEGIN: early-access-features -->/ {
    print; print ea_table; in_ea = 1; next;
  }
  /<!-- END: early-access-features -->/ {
    in_ea = 0; print; next;
  }

  # For beta features section
  /<!-- BEGIN: beta-features -->/ {
    print; print beta_table; in_beta = 1; next;
  }
  /<!-- END: beta-features -->/ {
    in_beta = 0; print; next;
  }

  # Skip lines between markers
  (in_exp || in_ea || in_beta) { next; }

  # Print all other lines
  { print; }
' "${dest}" >"${dest}.new"

# Move the new file into place
mv "${dest}.new" "${dest}"

(cd site && pnpm exec prettier --cache --write ../"${dest}")
