#!/usr/bin/env bash

set -euo pipefail

# Resolve paths before cd so they're absolute.
scriptdir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cd "$scriptdir/resources"
canonical_lock="$(pwd)/.terraform.lock.hcl"
manifest="$scriptdir/generation.sha1"

# kubernetes-metadata requires live infrastructure. duplicate-env-keys has
# repeated JSON keys whose UUIDs cannot be preserved by minimize_diff.
skip_module() {
	case "$1" in
	kubernetes-metadata | duplicate-env-keys) return 0 ;;
	*) return 1 ;;
	esac
}

terraform_version() {
	local version
	version="$(awk -F '"' '
		/^[[:space:]]*\[/ { tools = ($0 ~ /^[[:space:]]*\[tools\][[:space:]]*(#.*)?$/) }
		tools && /^[[:space:]]*terraform[[:space:]]*=[[:space:]]*"/ { print $2 }
	' "$scriptdir/../../../mise.toml")" || return
	if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
		echo "ERROR: mise.toml must pin tools.terraform to a release version." >&2
		return 1
	fi
	printf '%s\n' "$version"
}

# Content hashes avoid regeneration after checkout or a local CLI change.
fingerprint() {
	local file module i
	local lockfile="${1:-.terraform.lock.hcl}"
	local version
	version="$(terraform_version)" || return
	local -a files=(../generate.sh .terraform.lock.hcl)
	find . -type d \( -name .terraform -o -name .coder \) -prune -o \( -type f -o -type l \) -print0 |
		LC_ALL=C sort -z | {
		while IFS= read -r -d '' file; do
			module="${file#./}"
			module="${module%%/*}"
			if skip_module "$module"; then
				continue
			fi
			case "$file" in
			*.tfplan.json | *.tfstate.json | *.tfplan.dot | *.tfstate.dot | ./.terraform.lock.hcl | */.terraform.lock.hcl | *.tfstate | *.tfstate.* | *.tfplan | *.golden | *.md | */.gitignore) continue ;;
			esac
			files+=("$file")
		done
		printf 'terraform\0%s\0' "$version"
		printf '%s\0' "${files[@]}"
		for i in "${!files[@]}"; do
			[[ "${files[i]}" != .terraform.lock.hcl ]] || files[i]="$lockfile"
		done
		git hash-object --no-filters -- "${files[@]}"
	} | git hash-object --stdin
}

# These environment variables influence the coder provider.
for v in $(env | grep -E '^CODER_' | cut -d= -f1); do
	unset "$v"
done

generate() {
	local name="$1" ret

	echo "=== BEGIN: $name"
	if ((upgrade)); then
		terraform init -upgrade
	elif [[ -d "$scriptdir/resources/$name/.terraform/providers" ]]; then
		# An installed provider directory avoids registry version lookups.
		if ! terraform init -plugin-dir="$scriptdir/resources/$name/.terraform/providers"; then
			rm -rf .terraform/providers && terraform init
		fi
	else
		terraform init
	fi &&
		terraform plan -out terraform.tfplan &&
		terraform show -json ./terraform.tfplan | jq >"$name".tfplan.json &&
		terraform graph -type=plan >"$name".tfplan.dot &&
		rm terraform.tfplan &&
		terraform apply -auto-approve &&
		terraform show -json ./terraform.tfstate | jq >"$name".tfstate.json &&
		rm terraform.tfstate &&
		terraform graph -type=plan >"$name".tfstate.dot
	ret=$?
	echo "=== END: $name"
	return "$ret"
}

minimize_diff() {
	local name="$1"
	local f diff status line key value
	for f in *.tf*.json; do
		[[ -f "$scriptdir/resources/$name/$f" ]] || continue
		if diff="$(git diff --no-index -- "$scriptdir/resources/$name/$f" "$f")"; then
			continue
		else
			status=$?
			((status == 1)) || return "$status"
		fi
		declare -A deleted=()
		declare -a sed_args=()
		while IFS= read -r line; do
			[[ $line =~ \"(terraform_version|id|agent_id|subagent_id|resource_id|token|random|timestamp)\": ]] || continue
			# Deleted line (previous value).
			if [[ $line = -\ * ]]; then
				key="${line#*\"}"
				key="${key%%\"*}"
				value="${line#*: }"
				value="${value#*\"}"
				value="\"${value%\"*}\""
				declare deleted["$key"]="$value"
			# Added line (new value).
			elif [[ $line = +\ * ]]; then
				key="${line#*\"}"
				key="${key%%\"*}"
				value="${line#*: }"
				value="${value#*\"}"
				value="\"${value%\"*}\""
				# Matched key, restore the value.
				if [[ -v deleted["$key"] ]]; then
					sed_args+=(-e "s|${value}|${deleted["$key"]}|")
					unset "deleted[$key]"
				fi
			fi
			if [[ ${#sed_args[@]} -gt 0 ]]; then
				# Handle macOS compat.
				if grep -q -- "\[-i extension\]" < <(sed -h 2>&1); then
					sed -i '' "${sed_args[@]}" "$f"
				else
					sed -i'' "${sed_args[@]}" "$f"
				fi
			fi
		done <<<"$diff"
	done
}

# Extract the coder/coder provider version from the given lockfile.
# Two sed passes instead of nested brace blocks; BSD sed rejects
# them and would silently return an empty string on macOS.
extract_provider_version() {
	sed -n '/coder\/coder/,/^}/p' "$1" |
		sed -n 's/.*version[[:space:]]*=[[:space:]]*"\(.*\)".*/\1/p' |
		head -n 1
}

run() {
	local d="$1" name out
	cd "$d"
	name="${PWD##*/}"

	if skip_module "$name"; then
		echo "== Skipping hand-maintained fixture: $name"
		return 0
	fi

	echo "== Generating test data for: $name"
	if ! out="$(generate "$name" 2>&1)"; then
		echo "$out"
		echo "== Error generating test data for: $name"
		return 1
	fi
	if ((minimize)); then
		echo "== Minimizing diffs for: $name"
		minimize_diff "$name"
	fi
	mkdir -p "$workdir/result/resources/$name"
	mv "$name".tf{plan,state}.{json,dot} "$workdir/result/resources/$name/"
	if [[ -d .terraform/providers ]]; then
		mkdir -p "$workdir/providers/resources/$name"
		find .terraform/providers -type f -print0 | rsync -r --from0 --files-from=- ./ "$workdir/providers/resources/$name/"
	fi
	echo "== Done generating test data for: $name"
	return 0
}

if [[ " $* " == *" --help "* || " $* " == *" -h "* ]]; then
	echo "Usage: $0 [--upgrade] [--check] [--if-needed] [--no-minimize] [module1 module2 ...]"
	exit 0
fi

minimize=1
if [[ " $* " == *" --no-minimize "* ]]; then
	minimize=0
fi

upgrade=0
if [[ " $* " == *" --upgrade "* ]]; then
	upgrade=1
fi

if_needed=0
if [[ " $* " == *" --if-needed "* ]]; then
	if_needed=1
fi
check=0
if [[ " $* " == *" --check "* ]]; then
	check=1
fi

inputs="$(fingerprint)"
if ((check)); then
	expected="$(<"$scriptdir/provider-version.txt")"
	actual="$(extract_provider_version "$canonical_lock")"
	if [[ "$expected" != "$actual" ]]; then
		echo "ERROR: provider-version.txt ($expected) does not match lockfile ($actual)" >&2
		exit 1
	fi
fi
if ((check || if_needed)); then
	if [[ -f "$manifest" && "$(cat "$manifest")" == "$inputs" ]]; then
		exit 0
	fi
	if ((check)); then
		echo "Terraform fixture inputs changed; run make gen." >&2
		exit 1
	fi
	echo "== Terraform fixture inputs changed; regenerating"
fi

# Committed testdata encodes linux/amd64 values from coder_provisioner.
# Regenerating elsewhere bakes in the host OS/arch.
if [[ "$(uname)" != "Linux" ]]; then
	if ((upgrade)); then
		echo "ERROR: --upgrade is not supported on $(uname); run on Linux or via CI."
		exit 1
	fi
	echo "Note: skipping testdata regeneration on $(uname); regenerate on Linux or via CI."
	exit 0
fi

export CHECKPOINT_DISABLE=1
# Filter flags from positional args to get directory names.
declare -a dirs=()
for arg in "$@"; do
	case "$arg" in
	--upgrade | --no-minimize | --check | --if-needed | --help | -h) ;;
	*)
		module="$(cd "$arg" && pwd)"
		case "$module" in
		"$scriptdir/resources/"*) dirs+=("${module#"$scriptdir/resources/"}") ;;
		*)
			echo "ERROR: module must be within $scriptdir/resources: $arg" >&2
			exit 1
			;;
		esac
		;;
	esac
done

# Keep Terraform's working files and incomplete outputs out of the checkout.
workdir="$(mktemp -d "$scriptdir/.generate.XXXXXX")"
cleanup() {
	trap '' HUP INT TERM
	wait || true
	rm -rf "$workdir"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir -p "$workdir/result/resources" "$workdir/providers"
(
	cd "$scriptdir"
	tar --exclude='.terraform' --exclude='.coder' --exclude='*.tfstate' --exclude='*.tfstate.*' \
		--exclude='*.tfplan' -cf - resources generate.sh
) | tar -xf - -C "$workdir"
cd "$workdir/resources"
canonical_lock="$workdir/resources/.terraform.lock.hcl"
copied_inputs="$(fingerprint)"
if [[ "$copied_inputs" != "$inputs" ]]; then
	echo "ERROR: fixture inputs changed while copying; rerun generation." >&2
	exit 1
fi

full=0
if [[ ${#dirs[@]} -eq 0 ]]; then
	full=1
	dirs=(*/)
fi
for d in */; do
	cp "$canonical_lock" "$d/.terraform.lock.hcl"
done
declare -a jobs=()
for d in "${dirs[@]}"; do
	run "$d" &
	jobs+=($!)
done

err=0
for job in "${jobs[@]}"; do
	if ! wait "$job"; then
		err=$((err + 1))
	fi
done
if [[ $err -ne 0 ]]; then
	echo "ERROR: Failed to generate test data for $err modules"
	exit 1
fi

recorded_inputs="$inputs"
# After upgrade, promote the lockfile from a representative directory
# back to the canonical location and record the provider version.
if ((upgrade)); then
	# Prefer rich-parameters since it uses all providers (coder, null, docker).
	src=""
	if [[ -f "rich-parameters/.terraform.lock.hcl" ]]; then
		src="rich-parameters/.terraform.lock.hcl"
	else
		for d in */; do
			if [[ -f "$d/.terraform.lock.hcl" ]]; then
				src="$d/.terraform.lock.hcl"
				break
			fi
		done
	fi
	if [[ -n "$src" ]]; then
		cp "$src" "$canonical_lock"
		cp "$canonical_lock" "$workdir/result/resources/.terraform.lock.hcl"
		recorded_inputs="$(cd "$scriptdir/resources" && fingerprint "$canonical_lock")"
		version="$(extract_provider_version "$canonical_lock")"
		echo "$version" >"$workdir/result/provider-version.txt"
		echo "== Updated canonical lockfile and provider-version.txt (coder provider $version)"
	fi
fi

current_inputs="$(cd "$scriptdir/resources" && fingerprint)"
if [[ "$current_inputs" != "$inputs" ]]; then
	echo "ERROR: fixture inputs changed during generation; rerun generation." >&2
	exit 1
fi
rsync -rlt --ignore-existing "$workdir/providers/" "$scriptdir/"
rsync -rc --delay-updates "$workdir/result/" "$scriptdir/"
if ((full)); then
	# Record success only after all outputs have been copied.
	printf '%s\n' "$recorded_inputs" >"$workdir/generation.sha1"
	rsync -c "$workdir/generation.sha1" "$scriptdir/"
fi
