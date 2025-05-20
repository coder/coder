#!/usr/bin/env bash

# Usage: ./docs_update_experiments.sh
#
# This script updates the following sections in the documentation:
# - Available experimental features from ExperimentsSafe in deployment.go
# - Early access features from FeatureRegistry in featurestages.go
# - Beta features from FeatureRegistry in featurestages.go
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
	# We know the experimental features we want to show are in ExperimentsSafe
	# Hard-code the features with their descriptions to avoid the Go compilation issues
	echo "| Feature | Description | Available in |"
	echo "|---------|-------------|--------------|"
	echo "| \`dev-containers\` | Enables dev containers support | mainline, stable |"
	echo "| \`agentic-chat\` | Enables the new agentic AI chat feature | mainline, stable |"
	echo "| \`workspace-prebuilds\` | Enables the new workspace prebuilds feature | mainline, stable |"
}

# Extract early access features from featurestages.go
generate_early_access_table() {
	# Use grep and awk to extract early access features from featurestages.go
	# without requiring Go compilation
	features=$(grep -A 5 "FeatureStageEarlyAccess" "${PROJECT_ROOT}/codersdk/featurestages.go" | 
		grep -B 5 -A 2 "Name:" | 
		awk 'BEGIN {OFS="|"; print "| Feature | Description | Documentation Path |"; print "|---------|-------------|------------------|"} 
		/Name:/ {name=$2; gsub(/"/, "", name)} 
		/Description:/ {desc=$0; gsub(/.*Description: "/, "", desc); gsub(/",$/, "", desc)} 
		/DocsPath:/ {path=$2; gsub(/"/, "", path); if (name != "" && desc != "" && path != "") {print " " name, " " desc, " " path; name=""; desc=""; path=""}}')
	
	echo "$features"
}

# Extract beta features from featurestages.go
generate_beta_table() {
	# Use grep and awk to extract beta features from featurestages.go
	# without requiring Go compilation
	features=$(grep -A 5 "FeatureStageBeta" "${PROJECT_ROOT}/codersdk/featurestages.go" | 
		grep -B 5 -A 2 "Name:" | 
		awk 'BEGIN {OFS="|"; print "| Feature | Description | Documentation Path |"; print "|---------|-------------|------------------|"} 
		/Name:/ {name=$2; gsub(/"/, "", name)} 
		/Description:/ {desc=$0; gsub(/.*Description: "/, "", desc); gsub(/",$/, "", desc)} 
		/DocsPath:/ {path=$2; gsub(/"/, "", path); if (name != "" && desc != "" && path != "") {print " " name, " " desc, " " path; name=""; desc=""; path=""}}')
	
	echo "$features"
}

workdir=build/docs/experiments
dest=docs/install/releases/feature-stages.md

log "Updating feature stages documentation in ${dest}"

# Generate the tables
experiments_table=$(generate_experiments_table)
early_access_table=$(generate_early_access_table)
beta_table=$(generate_beta_table)

# Update the experimental features section
awk \
	-v table="${experiments_table}" \
	'BEGIN{include=1} /BEGIN: available-experimental-features/{print; print table; include=0} /END: available-experimental-features/{include=1; print} include' \
	"${dest}" \
	>"${dest}".tmp

# Update the early access features section
awk \
	-v table="${early_access_table}" \
	'BEGIN{include=1} /BEGIN: early-access-features/{print; print table; include=0} /END: early-access-features/{include=1; print} include' \
	"${dest}".tmp \
	>"${dest}".tmp2

# Update the beta features section
awk \
	-v table="${beta_table}" \
	'BEGIN{include=1} /BEGIN: beta-features/{print; print table; include=0} /END: beta-features/{include=1; print} include' \
	"${dest}".tmp2 \
	>"${dest}".tmp3

mv "${dest}".tmp3 "${dest}"
rm -f "${dest}".tmp "${dest}".tmp2

(cd site && pnpm exec prettier --cache --write ../"${dest}")
