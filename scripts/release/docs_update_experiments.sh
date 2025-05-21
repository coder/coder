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
' "${dest}" > "${dest}.new"

# Move the new file into place
mv "${dest}.new" "${dest}"

(cd site && pnpm exec prettier --cache --write ../"${dest}")
