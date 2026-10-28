#!/usr/bin/env bash

# Bumps the Terraform version bundled with Coder.
#
# Usage: ./scripts/update_terraform_version.sh [--skip-fixtures] <version>
#
# Run from the root of a coder/coder checkout. Updates every file that pins the
# bundled version (install.go, install.sh, Dockerfiles, mise, flake.nix, docs,
# setup-tf, testdata) and skips files a branch doesn't have, so it also works on
# older release branches. Checksums come from HashiCorp's GPG-verified
# SHA256SUMS. Unless --skip-fixtures is set, generation.sha1 is regenerated with
# the new binary. It doesn't source lib.sh, so CI can run main's copy against a
# release branch checkout.

set -euo pipefail

# HashiCorp's release signing key: https://www.hashicorp.com/trust/security
HASHICORP_KEY_URL="https://www.hashicorp.com/.well-known/pgp-key.txt"
HASHICORP_KEY_FINGERPRINT="C874011F0AB405110D02105534365D9472D7468F"
RELEASES_URL="https://releases.hashicorp.com/terraform"

semver_re='[0-9]+\.[0-9]+\.[0-9]+'

log() {
	echo "$*" >&2
}

error() {
	log "ERROR: $*"
	exit 1
}

usage() {
	log "Usage: $0 [--skip-fixtures] <version>"
	exit 64
}

# version_gt returns success if $1 is a strictly newer version than $2.
version_gt() {
	[[ "$1" != "$2" && "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -n 1)" == "$1" ]]
}

# replace_in_file applies sed expressions to a file in place. It avoids sed -i
# because GNU and BSD sed disagree on its syntax, and writes back through the
# existing file so its mode (e.g. install.sh's executable bit) is kept.
replace_in_file() {
	local file="$1" tmp
	shift
	tmp="$(mktemp)"
	sed -E "$@" "$file" >"$tmp"
	cat "$tmp" >"$file"
	rm -f "$tmp"
}

# hex_to_sri converts a hex SHA-256 digest to a Nix SRI hash.
hex_to_sri() {
	local hex="$1" i
	printf 'sha256-%s' "$(
		for ((i = 0; i < ${#hex}; i += 2)); do
			printf '%b' "\\x${hex:i:2}"
		done | base64 | tr -d '\n'
	)"
}

skip_fixtures=0
version=""
while (($#)); do
	case "$1" in
	--skip-fixtures) skip_fixtures=1 ;;
	-*) usage ;;
	*)
		[[ -z "$version" ]] || usage
		version="$1"
		;;
	esac
	shift
done
[[ -n "$version" ]] || usage
version="${version#v}"
[[ "$version" =~ ^${semver_re}$ ]] || error "version must look like 1.2.3, got: $version"

for cmd in curl gpg sed awk base64 sort; do
	command -v "$cmd" >/dev/null || error "$cmd is required"
done

install_go=provisioner/terraform/install.go
[[ -f "$install_go" ]] || error "run this from the root of a coder/coder checkout"

current="$(sed -nE "s/^[[:space:]]*TerraformVersion[[:space:]]*=[[:space:]]*version\.Must\(version\.NewVersion\(\"(${semver_re})\"\)\).*/\1/p" "$install_go")"
current_max="$(sed -nE "s/^[[:space:]]*maxTerraformVersion[[:space:]]*=[[:space:]]*version\.Must\(version\.NewVersion\(\"(${semver_re})\"\)\).*/\1/p" "$install_go")"
[[ -n "$current" && -n "$current_max" ]] || error "could not find TerraformVersion and maxTerraformVersion in $install_go"

if version_gt "$current" "$version"; then
	error "refusing to downgrade Terraform from $current to $version"
fi

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

# Download and verify the checksums for the new release.
sums="$workdir/terraform_${version}_SHA256SUMS"
log "Downloading and verifying terraform_${version}_SHA256SUMS"
if ! curl -fsSL --retry 3 -o "$sums" "$RELEASES_URL/$version/terraform_${version}_SHA256SUMS"; then
	error "Terraform $version was not found on releases.hashicorp.com"
fi
curl -fsSL --retry 3 -o "$sums.sig" "$RELEASES_URL/$version/terraform_${version}_SHA256SUMS.sig"
curl -fsSL --retry 3 -o "$workdir/hashicorp.asc" "$HASHICORP_KEY_URL"
export GNUPGHOME="$workdir/gnupg"
mkdir -m 700 "$GNUPGHOME"
gpg --batch --quiet --import "$workdir/hashicorp.asc" 2>/dev/null
if ! gpg --batch --status-fd 1 --verify "$sums.sig" "$sums" 2>/dev/null |
	grep -qE "^\[GNUPG:\] VALIDSIG .*${HASHICORP_KEY_FINGERPRINT}"; then
	error "terraform_${version}_SHA256SUMS is not signed by HashiCorp's key ($HASHICORP_KEY_FINGERPRINT)"
fi

# sha_for prints the hex digest of a release archive, e.g. sha_for linux_amd64.
sha_for() {
	awk -v f="terraform_${version}_$1.zip" '$2 == f { print $1 }' "$sums"
}

log "Bumping Terraform from $current to $version"
changed=()

# provisioner/terraform/install.go: maxTerraformVersion uses .9 to allow patch
# releases, so it only moves on a minor (or major) bump.
new_max="${version%.*}.9"
max_expr=()
if version_gt "$new_max" "$current_max"; then
	max_expr=(-e "s/^([[:space:]]*maxTerraformVersion[[:space:]]*=[[:space:]]*version\.Must\(version\.NewVersion\(\")${semver_re}/\1${new_max}/")
fi
replace_in_file "$install_go" \
	-e "s/^([[:space:]]*TerraformVersion[[:space:]]*=[[:space:]]*version\.Must\(version\.NewVersion\(\")${semver_re}/\1${version}/" \
	"${max_expr[@]}"
changed+=("$install_go")

# Download URLs, e.g. releases.hashicorp.com/terraform/1.2.3/terraform_1.2.3_
# in Dockerfiles, flake.nix and mise.lock.
url_expr="s#(releases\.hashicorp\.com/terraform/)${semver_re}/terraform_${semver_re}_#\1${version}/terraform_${version}_#g"

update() {
	local file="$1"
	shift
	[[ -f "$file" ]] || return 0
	replace_in_file "$file" -e "$url_expr" "$@"
	changed+=("$file")
}

update install.sh -e "s/^([[:space:]]*TERRAFORM_VERSION=\")${semver_re}\"/\1${version}\"/"
update scripts/Dockerfile.base
update dogfood/coder/Dockerfile -e "s/(Terraform version )${semver_re}/\1${version}/"
update docs/install/airgap.md -e "s/^(ARG TERRAFORM_VERSION=)${semver_re}/\1${version}/"
update docs/install/prepare/airgap.md -e "s/^(ARG TERRAFORM_VERSION=)${semver_re}/\1${version}/"
update .github/actions/setup-tf/action.yaml -e "s/^([[:space:]]*terraform_version:[[:space:]]*)${semver_re}/\1${version}/"
update mise.toml -e "s/^(terraform[[:space:]]*=[[:space:]]*\")${semver_re}\"/\1${version}\"/"

for f in provisioner/terraform/testdata/version.txt provisioner/terraform/testdata/resources/version.txt; do
	if [[ -f "$f" ]]; then
		printf '%s\n' "$version" >"$f"
		changed+=("$f")
	fi
done

# mise.lock: set the version and every platform checksum in the terraform
# entry. Each platform table pairs a checksum with the archive URL, which
# already points at the new release.
if [[ -f mise.lock ]]; then
	update mise.lock
	awk -v ver="$version" -v sums="$sums" '
		BEGIN {
			while ((getline line < sums) > 0) { split(line, f, /[[:space:]]+/); sha[f[2]] = f[1] }
		}
		function flush() {
			if (ck && file != "") {
				if (!(file in sha)) {
					print "ERROR: mise.lock references " file ", which is not in the SHA256SUMS" > "/dev/stderr"
					bad = 1
				} else {
					lines[ck] = "checksum = \"sha256:" sha[file] "\""
					updated++
				}
			}
			ck = 0
			file = ""
		}
		{ lines[NR] = $0 }
		/^\[/ {
			flush()
			in_tf = ($0 ~ /^\[\[tools\.terraform\]\]/ || $0 ~ /^\[tools\.terraform\./)
		}
		in_tf && /^version = "/ { lines[NR] = "version = \"" ver "\"" }
		in_tf && /^checksum = "/ { ck = NR }
		in_tf && /^url = "/ {
			file = $0
			sub(/^url = ".*\//, "", file)
			sub(/".*$/, "", file)
		}
		END {
			flush()
			if (bad) exit 1
			if (!updated) { print "ERROR: found no terraform checksums in mise.lock" > "/dev/stderr"; exit 1 }
			for (i = 1; i <= NR; i++) print lines[i]
		}
	' mise.lock >"$workdir/mise.lock"
	cat "$workdir/mise.lock" >mise.lock
fi

# flake.nix: fetchurl needs an SRI hash per platform. The hash follows either a
# `platform = "linux_amd64";` attribute or a URL naming the platform.
if [[ -f flake.nix ]] && grep -q 'releases\.hashicorp\.com/terraform/' flake.nix; then
	update flake.nix -e "s/(\"terraform-)${semver_re}\"/\1${version}\"/"
	sri_map="$workdir/sri"
	while read -r hex file; do
		platform="${file#terraform_"${version}"_}"
		platform="${platform%.zip}"
		printf '%s %s\n' "$platform" "$(hex_to_sri "$hex")"
	done < <(grep -E "terraform_${version//./\\.}_[a-z0-9]+_[a-z0-9]+\.zip$" "$sums") >"$sri_map"
	awk -v map="$sri_map" -v ver="$version" '
		BEGIN {
			while ((getline line < map) > 0) { split(line, f, " "); sri[f[1]] = f[2] }
		}
		{
			p = ""
			if (match($0, /platform = "[a-z0-9]+_[a-z0-9]+"/)) {
				p = substr($0, RSTART + 12, RLENGTH - 13)
			} else if (match($0, /terraform_[0-9.]+_[a-z0-9]+_[a-z0-9]+\.zip/)) {
				p = substr($0, RSTART, RLENGTH - 4)
				sub(/^terraform_[0-9.]+_/, "", p)
			}
			if (p != "" && (p in sri)) {
				pending = p
				age = 0
			} else if (pending != "" && match($0, /hash = "sha256-[A-Za-z0-9+\/=]+"/)) {
				sub(/hash = "sha256-[A-Za-z0-9+\/=]+"/, "hash = \"" sri[pending] "\"")
				pending = ""
				updated++
			} else if (pending != "" && ++age > 3) {
				pending = ""
			}
			print
		}
		END { if (!updated) { print "ERROR: found no terraform hashes in flake.nix" > "/dev/stderr"; exit 1 } }
	' flake.nix >"$workdir/flake.nix"
	cat "$workdir/flake.nix" >flake.nix
fi

# Sanity check: nothing we touched may still point at another release.
leftover="$(grep -nE "releases\.hashicorp\.com/terraform/${semver_re}" "${changed[@]}" | grep -vF "/terraform/${version}/" || true)"
if [[ -n "$leftover" ]]; then
	error "found download URLs for other Terraform releases:"$'\n'"$leftover"
fi
[[ "$(sed -nE "s/^[[:space:]]*TerraformVersion[[:space:]]*=[[:space:]]*version\.Must\(version\.NewVersion\(\"(${semver_re})\"\)\).*/\1/p" "$install_go")" == "$version" ]] ||
	error "failed to update TerraformVersion in $install_go"

# Regenerate the Terraform fixture fingerprint, which is keyed on the version
# in mise.toml, using the new Terraform binary.
if [[ -f provisioner/terraform/testdata/generation.sha1 ]]; then
	if ((skip_fixtures)); then
		log "Skipping fixture regeneration; run ./provisioner/terraform/testdata/generate.sh --if-needed with Terraform $version."
	elif [[ "$(uname -s)" != "Linux" ]]; then
		log "Skipping fixture regeneration on $(uname -s); run it on Linux or let CI tell you."
	else
		case "$(uname -m)" in
		x86_64) arch=amd64 ;;
		aarch64 | arm64) arch=arm64 ;;
		*) error "unsupported architecture for fixture regeneration: $(uname -m)" ;;
		esac
		zip="terraform_${version}_linux_${arch}.zip"
		log "Downloading $zip to regenerate fixtures"
		curl -fsSL --retry 3 -o "$workdir/$zip" "$RELEASES_URL/$version/$zip"
		actual="$( (sha256sum "$workdir/$zip" 2>/dev/null || shasum -a 256 "$workdir/$zip") | awk '{ print $1 }')"
		[[ "$actual" == "$(sha_for "linux_${arch}")" ]] || error "checksum mismatch for $zip"
		mkdir -p "$workdir/bin"
		unzip -q -o "$workdir/$zip" terraform -d "$workdir/bin"
		PATH="$workdir/bin:$PATH" ./provisioner/terraform/testdata/generate.sh --if-needed
	fi
fi

log "Bumped Terraform from $current to $version."
