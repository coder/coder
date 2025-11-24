#!/bin/bash
set -euo pipefail

# Linter to check for missing API docs files in manifest.json
# Cross-references docs/reference/api directory with manifest.json REST API node

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

API_DIR="docs/reference/api"
MANIFEST="docs/manifest.json"

echo "API Documentation Linter"
echo "========================"
echo ""

if [[ ! -d "$API_DIR" ]]; then
	echo -e "${RED}Error: API directory not found: $API_DIR${NC}"
	exit 1
fi

if [[ ! -f "$MANIFEST" ]]; then
	echo -e "${RED}Error: Manifest file not found: $MANIFEST${NC}"
	exit 1
fi

# Get all .md files in the API directory (excluding index.md)
echo "Scanning $API_DIR for markdown files..."
mapfile -t API_FILES < <(find "$API_DIR" -name "*.md" -not -name "index.md" -type f | sed 's|docs/|./|' | sort)

echo "Found ${#API_FILES[@]} API documentation files"
echo ""

# Extract paths from manifest.json REST API node
echo "Extracting paths from $MANIFEST (REST API node)..."
mapfile -t MANIFEST_PATHS < <(jq -r '.routes[] | select(.title == "Reference") | .children[] | select(.title == "REST API") | .children[] | .path' "$MANIFEST" | sort)

echo "Found ${#MANIFEST_PATHS[@]} entries in manifest"
echo ""

# Check for files in API dir that are missing from manifest
MISSING_FROM_MANIFEST=()
for file in "${API_FILES[@]}"; do
	found=false
	for manifest_path in "${MANIFEST_PATHS[@]}"; do
		if [[ "$manifest_path" == "$file" ]]; then
			found=true
			break
		fi
	done

	if [[ "$found" == "false" ]]; then
		MISSING_FROM_MANIFEST+=("$file")
	fi
done

# Check for manifest entries that don't have corresponding files
MISSING_FILES=()
for manifest_path in "${MANIFEST_PATHS[@]}"; do
	# Convert manifest path format (./reference/api/...) to filesystem path (docs/reference/api/...)
	fs_path="docs/${manifest_path#./}"

	if [[ ! -f "$fs_path" ]]; then
		MISSING_FILES+=("$manifest_path")
	fi
done

# Report results
ERROR_COUNT=0

if [[ ${#MISSING_FROM_MANIFEST[@]} -gt 0 ]]; then
	echo -e "${RED}✗ Files in $API_DIR missing from manifest.json:${NC}"
	for file in "${MISSING_FROM_MANIFEST[@]}"; do
		echo -e "  ${RED}- $file${NC}"
		ERROR_COUNT=$((ERROR_COUNT + 1))
	done
	echo ""
else
	echo -e "${GREEN}✓ All API files are listed in manifest.json${NC}"
	echo ""
fi

if [[ ${#MISSING_FILES[@]} -gt 0 ]]; then
	echo -e "${YELLOW}⚠ Manifest entries without corresponding files:${NC}"
	for path in "${MISSING_FILES[@]}"; do
		echo -e "  ${YELLOW}- $path${NC}"
		ERROR_COUNT=$((ERROR_COUNT + 1))
	done
	echo ""
else
	echo -e "${GREEN}✓ All manifest entries have corresponding files${NC}"
	echo ""
fi

# Summary
echo "Summary"
echo "-------"
echo "API files: ${#API_FILES[@]}"
echo "Manifest entries: ${#MANIFEST_PATHS[@]}"
echo "Missing from manifest: ${#MISSING_FROM_MANIFEST[@]}"
echo "Missing files: ${#MISSING_FILES[@]}"
echo ""

if [[ $ERROR_COUNT -gt 0 ]]; then
	echo -e "${RED}Linter found ${ERROR_COUNT} issue(s)${NC}"
	exit 1
else
	echo -e "${GREEN}✓ All checks passed!${NC}"
	exit 0
fi
