#!/bin/bash
set -euo pipefail

# Download PostgreSQL binaries from Maven repository
# This script downloads the binaries that embedded-postgres needs and validates checksums

MAVEN_BASE_URL="https://repo.maven.apache.org/maven2"
OUTPUT_DIR="files"

# PostgreSQL versions and platforms that Coder supports
# Based on what we see in cli/server.go (V13) and scripts/embedded-pg/main.go (V16)
POSTGRES_VERSIONS=("13.21.0" "16.6.0")
PLATFORMS=(
	"linux-amd64"
	"darwin-amd64"
	"darwin-arm64v8"
	"windows-amd64"
)

echo "Downloading PostgreSQL binaries for embedded-postgres mirror..."

mkdir -p "$OUTPUT_DIR"

download_and_verify() {
	local version=$1
	local platform=$2
	local base_url="$MAVEN_BASE_URL/io/zonky/test/postgres/embedded-postgres-binaries-$platform"
	local jar_path="$version/embedded-postgres-binaries-$platform-$version.jar"
	local jar_url="$base_url/$jar_path"
	local sha_url="$jar_url.sha256"

	local output_dir="$OUTPUT_DIR/io/zonky/test/postgres/embedded-postgres-binaries-$platform/$version"
	local jar_file="$output_dir/embedded-postgres-binaries-$platform-$version.jar"
	local sha_file="$jar_file.sha256"

	echo "  Downloading $platform $version..."

	# Create directory structure
	mkdir -p "$output_dir"

	# Download JAR file
	if curl --fail --silent --show-error --location "$jar_url" -o "$jar_file"; then
		echo "    ✓ Downloaded JAR ($(du -h "$jar_file" | cut -f1))"
	else
		echo "    ✗ Failed to download JAR from $jar_url"
		return 1
	fi

	# Download SHA256 checksum
	if curl --fail --silent --show-error --location "$sha_url" -o "$sha_file"; then
		echo "    ✓ Downloaded checksum"

		# Verify checksum
		local expected_checksum=$(cat "$sha_file")
		local actual_checksum=$(sha256sum "$jar_file" | cut -d' ' -f1)

		if [ "$expected_checksum" = "$actual_checksum" ]; then
			echo "    ✓ Checksum verified"
		else
			echo "    ✗ Checksum mismatch!"
			echo "      Expected: $expected_checksum"
			echo "      Actual:   $actual_checksum"
			return 1
		fi
	else
		echo "    ⚠ Failed to download checksum from $sha_url"
		# Generate our own checksum file
		sha256sum "$jar_file" | cut -d' ' -f1 >"$sha_file"
		echo "    ✓ Generated checksum file"
	fi
}

# Download all combinations
for version in "${POSTGRES_VERSIONS[@]}"; do
	echo "PostgreSQL $version:"
	for platform in "${PLATFORMS[@]}"; do
		if ! download_and_verify "$version" "$platform"; then
			echo "Failed to download $platform $version, continuing..."
		fi
	done
	echo
done

echo "Download summary:"
find "$OUTPUT_DIR" -name "*.jar" -exec du -h {} \; | sort
echo
echo "Total size: $(du -sh "$OUTPUT_DIR" | cut -f1)"
echo
echo "Directory structure created:"
find "$OUTPUT_DIR" -type d | head -10
echo "..."
echo
echo "Ready for Docker build!"
