#!/bin/bash

# This script produces a single markdown file from all of the docs content
# for the purpose of easy loading into an LLM UI such as Claude.

output_file="combined_docs.md"

# Clear the output file if it exists
: >"$output_file"

# Function to process markdown files
process_file() {
	local file="$1"
	cat <<EOF >>"$output_file"
## $(basename "$file" .md)

$(cat "$file")

---

EOF
}

# Find all markdown files in the docs directory and its subdirectories,
# excluding some files because we need the combined markdown under 128k tokens for Claude.
find . -name "*.md" ! -name "$(basename "$output_file")" ! -path "./reference/*" ! -path "./changelogs/*" | while read -r file; do
	process_file "$file"
done

echo "Combined markdown file created: $output_file"
echo "Words in file: $(wc -w <$output_file)"
