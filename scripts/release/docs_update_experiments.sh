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

# File path to deployment.go - needed for documentation purposes
# shellcheck disable=SC2034
DEPLOYMENT_GO_FILE="codersdk/deployment.go"

# Extract and parse experiment information from deployment.go
extract_experiment_info() {
	# Extract the experiment descriptions, stages, and doc paths
	# We'll use Go code to capture this information and print it in a structured format
	cat >/tmp/extract_experiment_info.go <<'EOT'
package main

import (
	"encoding/json"
	"os"

	"github.com/coder/coder/v2/codersdk"
)

func main() {
	experiments := []struct {
		Name        string `json:"name"`
		Value       string `json:"value"`
		Description string `json:"description"`
		Stage       string `json:"stage"`
	}{}

	// Get experiments from ExperimentsSafe
	for _, exp := range codersdk.ExperimentsSafe {
		experiments = append(experiments, struct {
			Name        string `json:"name"`
			Value       string `json:"value"`
			Description string `json:"description"`
			Stage       string `json:"stage"`
		}{
			Name:        string(exp),
			Value:       string(exp),
			Description: exp.GetDescription(),
			Stage:       string(exp.GetStage()),
		})
	}

	json.NewEncoder(os.Stdout).Encode(experiments)
}
EOT

	# Run the Go code to extract the information
	cd /home/coder/coder
	go run /tmp/extract_experiment_info.go
	rm /tmp/extract_experiment_info.go
}

# Generate the experimental features table with flag name
generate_experiments_table() {
	echo "| Feature Flag | Name | Available in |"
	echo "|-------------|------|--------------|"

	# Extract the experiment information
	extract_experiment_info | jq -r '.[] | select(.stage=="early access") | "| `\(.value)` | \(.description) | mainline, stable |"'
}

# Extract beta features from deployment.go
generate_beta_table() {
	echo "| Feature Flag | Name |"
	echo "|-------------|------|"

	# Extract beta features with flag name only
	extract_experiment_info | jq -r '.[] | select(.stage=="beta") | "| `\(.value)` | \(.description) |"'
}

dest=docs/install/releases/feature-stages.md

log "Updating feature stages documentation in ${dest}"

# Generate the tables
experiments_table=$(generate_experiments_table)
beta_table=$(generate_beta_table)

# We're using a single-pass awk script that replaces content between markers
# No need for cleanup operations

# Create temporary files with the new content
cat >/tmp/ea_content.md <<EOT
<!-- BEGIN: available-experimental-features -->

$experiments_table

<!-- END: available-experimental-features -->
EOT

cat >/tmp/beta_content.md <<EOT
<!-- BEGIN: beta-features -->

$beta_table

<!-- END: beta-features -->
EOT

# Use awk to replace the sections
awk '
  BEGIN { 
    ea = 0; beta = 0;
    while (getline < "/tmp/ea_content.md") ea_lines[++ea] = $0;
    while (getline < "/tmp/beta_content.md") beta_lines[++beta] = $0;
    ea = beta = 0;
  }
  
  /<!-- BEGIN: available-experimental-features -->/ { 
    for (i = 1; i <= length(ea_lines); i++) print ea_lines[i];
    ea = 1;
    next;
  }
  
  /<!-- END: available-experimental-features -->/ {
    ea = 0;
    next;
  }
  
  /<!-- BEGIN: beta-features -->/ {
    for (i = 1; i <= length(beta_lines); i++) print beta_lines[i];
    beta = 1;
    next;
  }
  
  /<!-- END: beta-features -->/ {
    beta = 0;
    next;
  }
  
  # Skip lines between markers
  (ea || beta) { next; }
  
  # Print all other lines
  { print; }
' "${dest}" >"${dest}.new"

# Move the new file into place
mv "${dest}.new" "${dest}"

# Clean up temporary files
rm -f /tmp/ea_content.md /tmp/beta_content.md

# Clean up backup files
rm -f "${dest}.bak"

# Format the file with prettier
(cd site && pnpm exec prettier --cache --write ../"${dest}")
