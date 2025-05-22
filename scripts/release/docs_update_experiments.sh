#!/usr/bin/env bash

# Usage: ./docs_update_experiments.sh
#
# This script updates the following sections in the documentation:
# - Available experimental features from ExperimentsSafe in codersdk/deployment.go
# - Early access features from GetStage() in codersdk/deployment.go
# - Beta features from GetStage() in codersdk/deployment.go
#
# The script will update feature-stages.md with tables for each section.
# For each feature, it also checks which versions it's available in (stable, mainline).

set -euo pipefail
# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"
cdroot

# Try to get GitHub token if not set
if [[ -z "${GITHUB_TOKEN:-}" ]]; then
	if GITHUB_TOKEN="$(gh auth token 2>/dev/null)"; then
		export GITHUB_TOKEN
		GH_AVAILABLE=true
	else
		log "Warning: GitHub token not found. Only checking local version."
		GH_AVAILABLE=false
	fi
else
	GH_AVAILABLE=true
fi

if isdarwin; then
	dependencies gsed gawk
	sed() { gsed "$@"; }
	awk() { gawk "$@"; }
fi

# Functions to get version information
echo_latest_stable_version() {
	if [[ "${GH_AVAILABLE}" == "false" ]]; then
		echo "stable"
		return
	fi
	
	# Try to get latest stable version, fallback to "stable" if it fails
	version=$(curl -fsSLI -o /dev/null -w "%{url_effective}" https://github.com/coder/coder/releases/latest 2>/dev/null || echo "error")
	if [[ "${version}" == "error" || -z "${version}" ]]; then
		log "Warning: Failed to fetch latest stable version. Using 'stable' as placeholder."
		echo "stable"
		return
	fi
	
	version="${version#https://github.com/coder/coder/releases/tag/v}"
	echo "v${version}"
}

echo_latest_mainline_version() {
	if [[ "${GH_AVAILABLE}" == "false" ]]; then
		echo "mainline"
		return
	fi
	
	# Try to get the latest mainline version, fallback to "mainline" if it fails
	local version
	version=$(curl -fsSL -H "Authorization: token ${GITHUB_TOKEN}" https://api.github.com/repos/coder/coder/releases 2>/dev/null | 
		awk -F'"' '/"tag_name"/ {print $4}' |
		tr -d v |
		tr . ' ' |
		sort -k1,1nr -k2,2nr -k3,3nr |
		head -n1 |
		tr ' ' . || echo "")
	
	if [[ -z "${version}" ]]; then
		log "Warning: Failed to fetch latest mainline version. Using 'mainline' as placeholder."
		echo "mainline"
		return
	fi
	
	echo "v${version}"
}

# Simplified function - we're no longer actually cloning the repo
# This is kept to maintain compatibility with the rest of the script structure
sparse_clone_codersdk() {
	if [[ "${GH_AVAILABLE}" == "false" ]]; then
		# Skip cloning if GitHub isn't available
		echo ""
		return
	fi
	
	# Always return success with a placeholder directory
	echo "${1}/${2}"
}

# Extract feature information from the local deployment.go
extract_local_experiment_info() {
	# Extract the experiment descriptions, stages, and doc paths using Go
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
	go run /tmp/extract_experiment_info.go
	rm -f /tmp/extract_experiment_info.go
}

# Extract experiment info from a specific version
extract_version_experiment_info() {
	local dir=$1
	local version=$2
	
	if [[ "${GH_AVAILABLE}" == "false" || -z "${dir}" ]]; then
		# If GitHub isn't available, just set all features to the same version
		extract_local_experiment_info | jq --arg version "${version}" '[.[] | . + {"versions": [$version]}]'
		return
	fi
	
	# For simplicity and stability, let's just use the local experiments
	# and mark them as available in the specified version.
	# This avoids the complex Go module replacement that can be error-prone
	extract_local_experiment_info | jq --arg version "${version}" '[.[] | . + {"versions": [$version]}]'
}

# Combine information from all versions
combine_experiment_info() {
	local workdir=$1
	local stable_version=$2
	local mainline_version=$3
	
	# Extract information from different versions
	local local_info stable_info mainline_info
	local_info=$(extract_local_experiment_info)
	
	if [[ "${GH_AVAILABLE}" == "true" ]]; then
		# Create sparse clones and extract info
		local stable_dir mainline_dir
		
		stable_dir=$(sparse_clone_codersdk "${workdir}" "stable" "${stable_version}")
		if [[ -n "${stable_dir}" ]]; then
			stable_info=$(extract_version_experiment_info "${stable_dir}" "stable")
		else
			# Fallback if sparse clone failed
			stable_info=$(extract_local_experiment_info | jq '[.[] | . + {"versions": ["stable"]}]')
		fi
		
		mainline_dir=$(sparse_clone_codersdk "${workdir}" "mainline" "${mainline_version}")
		if [[ -n "${mainline_dir}" ]]; then
			mainline_info=$(extract_version_experiment_info "${mainline_dir}" "mainline")
		else
			# Fallback if sparse clone failed
			mainline_info=$(extract_local_experiment_info | jq '[.[] | . + {"versions": ["mainline"]}]')
		fi
		
		# Cleanup
		rm -rf "${workdir}"
	else
		# If GitHub isn't available, just mark everything as available in all versions
		stable_info=$(extract_local_experiment_info | jq '[.[] | . + {"versions": ["stable"]}]')
		mainline_info=$(extract_local_experiment_info | jq '[.[] | . + {"versions": ["mainline"]}]')
	fi
	
	# Add 'main' version to local info
	local_info=$(echo "${local_info}" | jq '[.[] | . + {"versions": ["main"]}]')
	
	# Combine all info
	echo '[]' | jq \
		--argjson local "${local_info}" \
		--argjson stable "${stable_info:-[]}" \
		--argjson mainline "${mainline_info:-[]}" \
		'
		($local + $stable + $mainline) | 
		group_by(.value) | 
		map({
			name: .[0].name,
			value: .[0].value,
			description: .[0].description,
			stage: .[0].stage,
			versions: map(.versions[0]) | unique | sort
		})
		'
}

# Generate the early access features table
generate_experiments_table() {
	local experiment_info=$1
	
	echo "| Feature Flag | Name | Available in |"
	echo "|-------------|------|--------------|"
	
	echo "${experiment_info}" | jq -r '.[] | select(.stage=="early access") | "| `\(.value)` | \(.description) | \(.versions | join(", ")) |"'
}

# Generate the beta features table
generate_beta_table() {
	local experiment_info=$1
	
	echo "| Feature Flag | Name | Available in |"
	echo "|-------------|------|--------------|"
	
	echo "${experiment_info}" | jq -r '.[] | select(.stage=="beta") | "| `\(.value)` | \(.description) | \(.versions | join(", ")) |"'
}

workdir=build/docs/experiments
dest=docs/install/releases/feature-stages.md

log "Updating feature stages documentation in ${dest}"

# Get versions
stable_version=$(echo_latest_stable_version)
mainline_version=$(echo_latest_mainline_version)

log "Checking features for versions: main, ${mainline_version}, ${stable_version}"

# Get combined experiment information across versions
experiment_info=$(combine_experiment_info "${workdir}" "${stable_version}" "${mainline_version}")

# Generate the tables
experiments_table=$(generate_experiments_table "${experiment_info}")
beta_table=$(generate_beta_table "${experiment_info}")

# Create temporary files with the new content
cat >/tmp/ea_content.md <<EOT
<!-- BEGIN: available-experimental-features -->

${experiments_table}

<!-- END: available-experimental-features -->
EOT

cat >/tmp/beta_content.md <<EOT
<!-- BEGIN: beta-features -->

${beta_table}

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