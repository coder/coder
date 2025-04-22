#!/bin/sh
set -eu

# Coder's automatic install script.
# See https://github.com/coder/coder#install
#
# To run:
# curl -L https://coder.com/install.sh | sh

usage() {
	arg0="$0"
	if [ "$0" = sh ]; then
		arg0="curl -fsSL https://coder.com/install.sh | sh -s --"
	else
		not_curl_usage="The latest script is available at https://coder.com/install.sh
"
	fi

	cath <<EOF
Installs Coder.
It tries to use the system package manager if possible.
After successful installation it explains how to start Coder.

Pass in user@host to install Coder on user@host over ssh.
The remote host must have internet access.
${not_curl_usage-}
Usage:

  ${arg0} [--dry-run] [--mainline | --stable | --version X.X.X] [--method detect] \
        [--prefix ~/.local] [--rsh ssh] [user@host]

  --dry-run
      Echo the commands for the install process without running them.

  --mainline
      Install the latest mainline version (default).

  --stable
	  Install the latest stable version instead of the latest mainline version.

  --version X.X.X
      Install a specific version instead of the latest.

  --method [detect | standalone]
      Choose the installation method. Defaults to detect.
      - detect detects the system package manager and tries to use it.
        Full reference on the process is further below.
      - standalone installs a standalone release archive into /usr/local/bin

  --prefix <dir>
      Sets the prefix used by standalone release archives. Defaults to /usr/local
      and the binary is copied into /usr/local/bin
      To install in \$HOME, pass --prefix=\$HOME/.local

  --binary-name <name>
	  Sets the name for the CLI in standalone release archives. Defaults to "coder"
	  To use the CLI as coder2, pass --binary-name=coder2
	  Note: in-product documentation will always refer to the CLI as "coder"

  --rsh <bin>
      Specifies the remote shell for remote installation. Defaults to ssh.

  --with-terraform
	  Installs Terraform binary from https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/ source
	  alongside coder.
	  This is great for if you are having issues with Coder installing terraform, or if you
	  just want it on your base system aswell.
	  This supports most systems, however if you are unsure yours is supported you can check
	  the link above.
  --net-admin
	  Adds \`CAP_NET_ADMIN\` to the installed binary. This allows Coder to
	  increase network speeds, but has security implications.
	  See: https://man7.org/linux/man-pages/man7/capabilities.7.html
	  This only works on Linux based systems.


The detection method works as follows:
  - Debian, Ubuntu, Raspbian: install the deb package from GitHub.
  - Fedora, CentOS, RHEL, openSUSE: install the rpm package from GitHub.
  - Alpine: install the apk package from GitHub.
  - macOS:  if \`brew\` is available, install from the coder/coder Homebrew tap.
  - Otherwise, download from GitHub and install into \`--prefix\`.

We build releases on GitHub for amd64, armv7, and arm64 on Windows, Linux, and macOS.

When the detection method tries to pull a release from GitHub it will
fall back to installing standalone when there is no matching release for
the system's operating system and architecture.

The installer will cache all downloaded assets into ~/.cache/coder
EOF
}

echo_latest_stable_version() {
	url="https://github.com/coder/coder/releases/latest"
	# https://gist.github.com/lukechilds/a83e1d7127b78fef38c2914c4ececc3c#gistcomment-2758860
	response=$(curl -sSLI -o /dev/null -w "\n%{http_code} %{url_effective}" ${url})
	status_code=$(echo "$response" | tail -n1 | cut -d' ' -f1)
	version=$(echo "$response" | tail -n1 | cut -d' ' -f2-)
	body=$(echo "$response" | sed '$d')

	if [ "$status_code" != "200" ]; then
		echoerr "GitHub API returned status code: ${status_code}"
		echoerr "URL: ${url}"
		exit 1
	fi

	version="${version#https://github.com/coder/coder/releases/tag/v}"
	echo "${version}"
}

echo_latest_mainline_version() {
	# Fetch the releases from the GitHub API, sort by version number,
	# and take the first result. Note that we're sorting by space-
	# separated numbers and without utilizing the sort -V flag for the
	# best compatibility.
	url="https://api.github.com/repos/coder/coder/releases"
	response=$(curl -sSL -w "\n%{http_code}" ${url})
	status_code=$(echo "$response" | tail -n1)
	body=$(echo "$response" | sed '$d')

	if [ "$status_code" != "200" ]; then
		echoerr "GitHub API returned status code: ${status_code}"
		echoerr "URL: ${url}"
		echoerr "Response body: ${body}"
		exit 1
	fi

	echo "$body" |
		awk -F'"' '/"tag_name"/ {print $4}' |
		tr -d v |
		tr . ' ' |
		sort -k1,1nr -k2,2nr -k3,3nr |
		head -n1 |
		tr ' ' .
}

echo_standalone_postinstall() {
	if [ "${DRY_RUN-}" ]; then
		echo_dryrun_postinstall
		return
	fi

	channel=
	advisory="To install our stable release (v${STABLE_VERSION}), use the --stable flag. "
	if [ "${STABLE}" = 1 ]; then
		channel="stable "
		advisory=""
	fi
	if [ "${MAINLINE}" = 1 ]; then
		channel="mainline "
	fi

	cath <<EOF

Coder ${channel}release v${VERSION} installed. ${advisory}See our releases documentation or GitHub for more information on versioning.

The Coder binary has been placed in the following location:

  $STANDALONE_INSTALL_PREFIX/bin/$STANDALONE_BINARY_NAME

EOF

	CODER_COMMAND="$(command -v "$STANDALONE_BINARY_NAME" || true)"

	if [ -z "${CODER_COMMAND}" ]; then
		cath <<EOF
Extend your path to use Coder:

  $ PATH="$STANDALONE_INSTALL_PREFIX/bin:\$PATH"

EOF
	elif [ "$CODER_COMMAND" != "$STANDALONE_BINARY_LOCATION" ]; then
		echo_path_conflict "$CODER_COMMAND"
	else
		cath <<EOF
To run a Coder server:

  $ $STANDALONE_BINARY_NAME server

To connect to a Coder deployment:

  $ $STANDALONE_BINARY_NAME login <deployment url>

EOF
	fi
}

echo_brew_postinstall() {
	if [ "${DRY_RUN-}" ]; then
		echo_dryrun_postinstall
		return
	fi

	BREW_PREFIX="$(brew --prefix)"

	cath <<EOF

Coder has been installed to

  $BREW_PREFIX/bin/coder

EOF

	CODER_COMMAND="$(command -v "coder" || true)"

	if [ "$CODER_COMMAND" != "$BREW_PREFIX/bin/coder" ]; then
		echo_path_conflict "$CODER_COMMAND"
	fi

	cath <<EOF
To run a Coder server:

  $ coder server

To connect to a Coder deployment:

  $ coder login <deployment url>

EOF
}

echo_systemd_postinstall() {
	if [ "${DRY_RUN-}" ]; then
		echo_dryrun_postinstall
		return
	fi

	echoh
	cath <<EOF
$1 package has been installed.

To run a Coder server:

  # Start Coder now and on reboot
  $ sudo systemctl enable --now coder
  $ journalctl -u coder.service -b

  # Or just run the server directly
  $ coder server

  Configuring Coder: https://coder.com/docs/admin/setup

To connect to a Coder deployment:

  $ coder login <deployment url>

EOF
}

echo_dryrun_postinstall() {
	cath <<EOF
Dry-run complete.

To install Coder, re-run this script without the --dry-run flag.

EOF
}

echo_path_conflict() {
	cath <<EOF
There is another binary in your PATH that conflicts with the binary we've installed.

  $1

This is likely because of an existing installation of Coder in your \$PATH.

Run \`which -a coder\` to view all installations.

EOF
}

main() {
	MAINLINE=1
	STABLE=0
	TERRAFORM_VERSION="1.11.4"

	if [ "${TRACE-}" ]; then
		set -x
	fi

	unset \
		DRY_RUN \
		METHOD \
		OPTIONAL \
		ALL_FLAGS \
		RSH_ARGS \
		RSH \
		WITH_TERRAFORM \
		CAP_NET_ADMIN

	ALL_FLAGS=""

	while [ "$#" -gt 0 ]; do
		case "$1" in
		-*)
			ALL_FLAGS="${ALL_FLAGS} $1"
			;;
		esac

		case "$1" in
		--dry-run)
			DRY_RUN=1
			;;
		--method)
			METHOD="$(parse_arg "$@")"
			shift
			;;
		--method=*)
			METHOD="$(parse_arg "$@")"
			;;
		--prefix)
			STANDALONE_INSTALL_PREFIX="$(parse_arg "$@")"
			shift
			;;
		--prefix=*)
			STANDALONE_INSTALL_PREFIX="$(parse_arg "$@")"
			;;
		--binary-name)
			STANDALONE_BINARY_NAME="$(parse_arg "$@")"
			shift
			;;
		--binary-name=*)
			STANDALONE_BINARY_NAME="$(parse_arg "$@")"
			;;
		--version)
			VERSION="$(parse_arg "$@")"
			MAINLINE=0
			STABLE=0
			shift
			;;
		--version=*)
			VERSION="$(parse_arg "$@")"
			MAINLINE=0
			STABLE=0
			;;
		# Support edge for backward compatibility.
		--mainline | --edge)
			VERSION=
			MAINLINE=1
			STABLE=0
			;;
		--stable)
			VERSION=
			MAINLINE=0
			STABLE=1
			;;
		--rsh)
			RSH="$(parse_arg "$@")"
			shift
			;;
		--rsh=*)
			RSH="$(parse_arg "$@")"
			;;
		-h | --h | -help | --help)
			usage
			exit 0
			;;
		--with-terraform)
			WITH_TERRAFORM=1
			;;
		--net-admin)
			CAP_NET_ADMIN=1
			;;
		--)
			shift
			# We remove the -- added above.
			ALL_FLAGS="${ALL_FLAGS% --}"
			RSH_ARGS="$*"
			break
			;;
		-*)
			echoerr "Unknown flag $1"
			echoerr "Run with --help to see usage."
			exit 1
			;;
		*)
			RSH_ARGS="$*"
			break
			;;
		esac

		shift
	done

	if [ "${RSH_ARGS-}" ]; then
		RSH="${RSH-ssh}"
		echoh "Installing remotely with $RSH $RSH_ARGS"
		curl -fsSL https://coder.com/install.sh | prefix "$RSH_ARGS" "$RSH" "$RSH_ARGS" sh -s -- "$ALL_FLAGS"
		return
	fi

	# These can be overridden for testing but shouldn't normally be used as it can
	# result in a broken coder.
	OS=${OS:-$(os)}
	ARCH=${ARCH:-$(arch)}
	TERRAFORM_ARCH=${TERRAFORM_ARCH:-$(terraform_arch)}

	# If we've been provided a flag which is specific to the standalone installation
	# method, we should "detect" standalone to be the appropriate installation method.
	# This check needs to occur before we set these variables with defaults.
	if [ "${STANDALONE_INSTALL_PREFIX-}" ] || [ "${STANDALONE_BINARY_NAME-}" ]; then
		METHOD=standalone
	fi

	METHOD="${METHOD-detect}"
	if [ "$METHOD" != detect ] && [ "$METHOD" != standalone ]; then
		echoerr "Unknown install method \"$METHOD\""
		echoerr "Run with --help to see usage."
		exit 1
	fi

	# We can't reasonably support installing specific versions of Coder through
	# Homebrew, so if we're on macOS and the `--version` flag or the `--stable`
	# flag (our tap follows mainline) was set, we should "detect" standalone to
	# be the appropriate installation method. This check needs to occur before we
	# set `VERSION` to a default of the latest release.
	if [ "$OS" = "darwin" ] && { [ "${VERSION-}" ] || [ "${STABLE}" = 1 ]; }; then
		METHOD=standalone
	fi

	# These are used by the various install_* functions that make use of GitHub
	# releases in order to download and unpack the right release.
	CACHE_DIR=$(echo_cache_dir)
	TERRAFORM_INSTALL_PREFIX=${TERRAFORM_INSTALL_PREFIX:-/usr/local}
	STANDALONE_INSTALL_PREFIX=${STANDALONE_INSTALL_PREFIX:-/usr/local}
	STANDALONE_BINARY_NAME=${STANDALONE_BINARY_NAME:-coder}
	STABLE_VERSION=$(echo_latest_stable_version)
	if [ "${MAINLINE}" = 1 ]; then
		VERSION=$(echo_latest_mainline_version)
		echoh "Resolved mainline version: v${VERSION}"
	elif [ "${STABLE}" = 1 ]; then
		VERSION=${STABLE_VERSION}
		echoh "Resolved stable version: v${VERSION}"
	fi

	distro_name

	if [ "${DRY_RUN-}" ]; then
		echoh "Running with --dry-run; the following are the commands that would be run if this were a real installation:"
		echoh
	fi

	# Start by installing Terraform, if requested
	if [ "${WITH_TERRAFORM-}" ]; then
		with_terraform
	fi

	# If the version is the same as the stable version, we're installing
	# the stable version.
	if [ "${MAINLINE}" = 1 ] && [ "${VERSION}" = "${STABLE_VERSION}" ]; then
		echoh "The latest mainline version has been promoted to stable, selecting stable."
		MAINLINE=0
		STABLE=1
	fi
	# If the manually specified version is stable, mark it as such.
	if [ "${MAINLINE}" = 0 ] && [ "${STABLE}" = 0 ] && [ "${VERSION}" = "${STABLE_VERSION}" ]; then
		STABLE=1
	fi

	# Standalone installs by pulling pre-built releases from GitHub.
	if [ "$METHOD" = standalone ]; then
		if has_standalone; then
			install_standalone
			exit 0
		else
			echoerr "There are no standalone releases for $ARCH"
			echoerr "Please try again without '--method standalone'"
			exit 1
		fi
	fi

	# DISTRO can be overridden for testing but shouldn't normally be used as it
	# can result in a broken coder.
	DISTRO=${DISTRO:-$(distro)}

	case $DISTRO in
	darwin) install_macos ;;
	# The .deb and .rpm files are pulled from GitHub.
	debian) install_deb ;;
	fedora | opensuse) install_rpm ;;
	# We don't have GitHub releases that work on Alpine or FreeBSD so we have no
	# choice but to use npm here.
	alpine) install_apk ;;
	# For anything else we'll try to install standalone but fall back to npm if
	# we don't have releases for the architecture.
	*)
		echoh "Unsupported package manager."
		echoh "Falling back to standalone installation."
		install_standalone
		;;
	esac

	if [ "${CAP_NET_ADMIN:-}" ]; then
		cap_net_admin
	fi
}

cap_net_admin() {
	if ! command_exists setcap && command_exists capsh; then
		echo "Package 'libcap' not found. See install instructions for your distro: https://command-not-found.com/setcap"
		return
	fi

	# Make sure we'e allowed to add CAP_NET_ADMIN.
	if sudo_sh_c capsh --has-p=CAP_NET_ADMIN; then
		sudo_sh_c setcap CAP_NET_ADMIN=+ep "$(command -v coder)" || true

	# Unable to escalate perms, notify the user.
	else
		echo "Unable to setcap agent binary. Ensure the root user has CAP_NET_ADMIN permissions."
	fi
}

parse_arg() {
	case "$1" in
	*=*)
		# Remove everything after first equal sign.
		opt="${1%%=*}"
		# Remove everything before first equal sign.
		optarg="${1#*=}"
		if [ ! "$optarg" ] && [ ! "${OPTIONAL-}" ]; then
			echoerr "$opt requires an argument"
			echoerr "Run with --help to see usage."
			exit 1
		fi
		echo "$optarg"
		return
		;;
	esac

	case "${2-}" in
	"" | -*)
		if [ ! "${OPTIONAL-}" ]; then
			echoerr "$1 requires an argument"
			echoerr "Run with --help to see usage."
			exit 1
		fi
		;;
	*)
		echo "$2"
		return
		;;
	esac
}

fetch() {
	URL="$1"
	FILE="$2"

	if [ -e "$FILE" ]; then
		echoh "+ Reusing $FILE"
		return
	fi

	sh_c mkdir -p "$CACHE_DIR"
	sh_c curl \
		-#fL \
		-o "$FILE.incomplete" \
		-C - \
		"$URL"
	sh_c mv "$FILE.incomplete" "$FILE"
}

with_terraform() {
	# Check if the unzip package is installed. If not error peacefully.
	if ! (command_exists unzip); then
		echoh
		echoerr "This script needs the unzip package to run."
		echoerr "Please install unzip to use this function"
		exit 1
	fi
	echoh "Installing Terraform version $TERRAFORM_VERSION $TERRAFORM_ARCH from the HashiCorp release repository."
	echoh

	# Download from official source and save it to cache
	fetch "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_${OS}_${TERRAFORM_ARCH}.zip" \
		"$CACHE_DIR/terraform_${TERRAFORM_VERSION}_${OS}_${TERRAFORM_ARCH}.zip"

	sh_c mkdir -p "$TERRAFORM_INSTALL_PREFIX" 2>/dev/null || true

	sh_c="sh_c"
	if [ ! -w "$TERRAFORM_INSTALL_PREFIX" ]; then
		sh_c="sudo_sh_c"
	fi
	# Prepare /usr/local/bin/ and the binary for copying
	"$sh_c" mkdir -p "$TERRAFORM_INSTALL_PREFIX/bin"
	"$sh_c" unzip -d "$CACHE_DIR" -o "$CACHE_DIR/terraform_${TERRAFORM_VERSION}_${OS}_${ARCH}.zip"
	COPY_LOCATION="$TERRAFORM_INSTALL_PREFIX/bin/terraform"

	# Remove the file if it already exists to
	# avoid https://github.com/coder/coder/issues/2086
	if [ -f "$COPY_LOCATION" ]; then
		"$sh_c" rm "$COPY_LOCATION"
	fi

	# Copy the binary to the correct location.
	"$sh_c" cp "$CACHE_DIR/terraform" "$COPY_LOCATION"
}

install_macos() {
	# If there is no `brew` binary available, just default to installing standalone
	if command_exists brew; then
		echoh "Installing coder with Homebrew from the coder/coder tap."
		echoh

		sh_c brew install coder/coder/coder

		echo_brew_postinstall
		return
	fi

	echoh "Homebrew is not available."
	echoh "Falling back to standalone installation."
	install_standalone
}

install_deb() {
	echoh "Installing v$VERSION of the $ARCH deb package from GitHub."
	echoh

	fetch "https://github.com/coder/coder/releases/download/v$VERSION/coder_${VERSION}_${OS}_${ARCH}.deb" \
		"$CACHE_DIR/coder_${VERSION}_$ARCH.deb"
	sudo_sh_c dpkg --force-confdef --force-confold -i "$CACHE_DIR/coder_${VERSION}_$ARCH.deb"

	echo_systemd_postinstall deb
}

install_rpm() {
	echoh "Installing v$VERSION of the $ARCH rpm package from GitHub."
	echoh

	fetch "https://github.com/coder/coder/releases/download/v$VERSION/coder_${VERSION}_${OS}_${ARCH}.rpm" \
		"$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.rpm"
	sudo_sh_c rpm -U "$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.rpm"

	echo_systemd_postinstall rpm
}

install_apk() {
	echoh "Installing v$VERSION of the $ARCH apk package from GitHub."
	echoh

	fetch "https://github.com/coder/coder/releases/download/v$VERSION/coder_${VERSION}_${OS}_${ARCH}.apk" \
		"$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.apk"
	sudo_sh_c apk add --allow-untrusted "$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.apk"

	echo_systemd_postinstall apk
}

install_standalone() {
	echoh "Installing v$VERSION of the $ARCH release from GitHub."
	echoh

	# macOS releases are packaged as .zip
	case $OS in
	darwin) STANDALONE_ARCHIVE_FORMAT=zip ;;
	*) STANDALONE_ARCHIVE_FORMAT=tar.gz ;;
	esac

	fetch "https://github.com/coder/coder/releases/download/v$VERSION/coder_${VERSION}_${OS}_${ARCH}.$STANDALONE_ARCHIVE_FORMAT" \
		"$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.$STANDALONE_ARCHIVE_FORMAT"

	# -w only works if the directory exists so try creating it first. If this
	# fails we can ignore the error as the -w check will then swap us to sudo.
	sh_c mkdir -p "$STANDALONE_INSTALL_PREFIX" 2>/dev/null || true

	sh_c mkdir -p "$CACHE_DIR/tmp"
	if [ "$STANDALONE_ARCHIVE_FORMAT" = tar.gz ]; then
		sh_c tar -C "$CACHE_DIR/tmp" -xzf "$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.tar.gz"
	else
		sh_c unzip -d "$CACHE_DIR/tmp" -o "$CACHE_DIR/coder_${VERSION}_${OS}_${ARCH}.zip"
	fi

	STANDALONE_BINARY_LOCATION="$STANDALONE_INSTALL_PREFIX/bin/$STANDALONE_BINARY_NAME"

	sh_c="sh_c"
	if [ ! -w "$STANDALONE_INSTALL_PREFIX" ]; then
		sh_c="sudo_sh_c"
	fi

	"$sh_c" mkdir -p "$STANDALONE_INSTALL_PREFIX/bin"

	# Remove the file if it already exists to
	# avoid https://github.com/coder/coder/issues/2086
	if [ -f "$STANDALONE_BINARY_LOCATION" ]; then
		"$sh_c" rm "$STANDALONE_BINARY_LOCATION"
	fi

	# Copy the binary to the correct location.
	"$sh_c" cp "$CACHE_DIR/tmp/coder" "$STANDALONE_BINARY_LOCATION"

	# Clean up the extracted files (note, not using sudo: $sh_c -> sh_c).
	sh_c rm -rv "$CACHE_DIR/tmp"

	echo_standalone_postinstall
}

# Determine if we have standalone releases on GitHub for the system's arch.
has_standalone() {
	case $ARCH in
	amd64) return 0 ;;
	arm64) return 0 ;;
	armv7)
		[ "$(distro)" != darwin ]
		return
		;;
	*) return 1 ;;
	esac
}

os() {
	uname="$(uname)"
	case $uname in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	FreeBSD) echo freebsd ;;
	*) echo "$uname" ;;
	esac
}

# Print the detected Linux distro, otherwise print the OS name.
#
# Example outputs:
# - darwin -> darwin
# - freebsd -> freebsd
# - ubuntu, raspbian, debian ... -> debian
# - amzn, centos, rhel, fedora, ... -> fedora
# - opensuse-{leap,tumbleweed} -> opensuse
# - alpine -> alpine
# - arch -> arch
#
# Inspired by https://github.com/docker/docker-install/blob/26ff363bcf3b3f5a00498ac43694bf1c7d9ce16c/install.sh#L111-L120.
distro() {
	if [ "$OS" = "darwin" ] || [ "$OS" = "freebsd" ]; then
		echo "$OS"
		return
	fi

	if [ -f /etc/os-release ]; then
		(
			# shellcheck disable=SC1091
			. /etc/os-release
			if [ "${ID_LIKE-}" ]; then
				for id_like in $ID_LIKE; do
					case "$id_like" in debian | fedora | opensuse)
						echo "$id_like"
						return
						;;
					esac
				done
			fi

			echo "$ID"
		)
		return
	fi
}

# Print a human-readable name for the OS/distro.
distro_name() {
	if [ "$(uname)" = "Darwin" ]; then
		echo "macOS v$(sw_vers -productVersion)"
		return
	fi

	if [ -f /etc/os-release ]; then
		(
			# shellcheck disable=SC1091
			. /etc/os-release
			echo "$PRETTY_NAME"
		)
		return
	fi

	# Prints something like: Linux 4.19.0-9-amd64
	uname -sr
}

arch() {
	uname_m=$(uname -m)
	case $uname_m in
	aarch64) echo arm64 ;;
	x86_64) echo amd64 ;;
	armv7l) echo armv7 ;;
	*) echo "$uname_m" ;;
	esac
}

# The following is to change the naming, that way people with armv7 won't receive a error
# List of binaries can be found here: https://releases.hashicorp.com/terraform/
terraform_arch() {
	uname_m=$(uname -m)
	case $uname_m in
	aarch64) echo arm64 ;;
	x86_64) echo amd64 ;;
	armv7l) echo arm ;;
	*) echo "$uname_m" ;;
	esac
}

command_exists() {
	if [ ! "$1" ]; then return 1; fi
	command -v "$@" >/dev/null
}

sh_c() {
	echoh "+ $*"
	if [ ! "${DRY_RUN-}" ]; then
		sh -c "$*"
	fi
}

sudo_sh_c() {
	if [ "$(id -u)" = 0 ]; then
		sh_c "$@"
	elif command_exists sudo; then
		sh_c "sudo $*"
	elif command_exists doas; then
		sh_c "doas $*"
	elif command_exists su; then
		sh_c "su - -c '$*'"
	else
		echoh
		echoerr "This script needs to run the following command as root."
		echoerr "  $*"
		echoerr "Please install sudo, su, or doas."
		exit 1
	fi
}

echo_cache_dir() {
	if [ "${XDG_CACHE_HOME-}" ]; then
		echo "$XDG_CACHE_HOME/coder"
	elif [ "${HOME-}" ]; then
		echo "$HOME/.cache/coder"
	else
		echo "/tmp/coder-cache"
	fi
}

echoh() {
	echo "$@" | humanpath
}

cath() {
	humanpath
}

echoerr() {
	echoh "$@" >&2
}

# humanpath replaces all occurrences of " $HOME" with " ~"
# and all occurrences of '"$HOME' with the literal '"$HOME'.
humanpath() {
	sed "s# $HOME# ~#g; s#\"$HOME#\"\$HOME#g"
}

# We need to make sure we exit with a non zero exit if the command fails.
# /bin/sh does not support -o pipefail unfortunately.
prefix() {
	PREFIX="$1"
	shift
	fifo="$(mktemp -d)/fifo"
	mkfifo "$fifo"
	sed -e "s#^#$PREFIX: #" "$fifo" &
	"$@" >"$fifo" 2>&1
}

main "$@"
