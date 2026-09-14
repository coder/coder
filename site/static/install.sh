#!/bin/sh
set -eu

# Coder's automatic install script.
# See https://github.com/coder/coder#install
#
# To run:
# curl -fsSL "{{ .Origin }}/install.sh" | sh

usage() {
	arg0="$0"
	if [ "$0" = sh ]; then
		arg0="curl -fsSL \"{{ .Origin }}/install.sh\" | sh -s --"
	else
		not_curl_usage="The latest script is available at {{ .Origin }}/install.sh
"
	fi

	cath <<EOF
Installs the Coder CLI.
A matching version of the CLI will be downloaded from this Coder deployment.

Pass in user@host to install the CLI on user@host over ssh.
The remote host must have internet access.
${not_curl_usage-}
Usage:

  ${arg0} [--dry-run] [--prefix ~/.local] [--rsh ssh] [user@host]

  --dry-run
      Echo the commands for the install process without running them.

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


We build releases on GitHub for amd64 and arm64 on Windows, Linux, and macOS, as
well as armv7 on Linux.

The installer will cache all downloaded assets into ~/.cache/coder
EOF
}

echo_standalone_postinstall() {
	if [ "${DRY_RUN-}" ]; then
		echo_dryrun_postinstall
		return
	fi

	cath <<EOF

Coder {{ .Version }} installed.

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
	if [ "${TRACE-}" ]; then
		set -x
	fi

	unset \
		DRY_RUN \
		ORIGIN \
		ALL_FLAGS \
		RSH_ARGS \
		RSH

	ORIGIN="{{ .Origin }}"
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
		--origin)
			ORIGIN="$(parse_arg "$@")"
			shift
			;;
		--origin=*)
			ORIGIN="$(parse_arg "$@")"
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
		curl -fsSL "$ORIGIN/install.sh" | prefix "$RSH_ARGS" "$RSH" "$RSH_ARGS" sh -s -- "$ALL_FLAGS"
		return
	fi

	# These can be overridden for testing but shouldn't normally be used as it can
	# result in a broken coder.
	OS=${OS:-$(os)}
	ARCH=${ARCH:-$(arch)}

	# These are used by the various install_* functions that make use of GitHub
	# releases in order to download and unpack the right release.
	CACHE_DIR=$(echo_cache_dir)
	STANDALONE_INSTALL_PREFIX=${STANDALONE_INSTALL_PREFIX:-/usr/local}
	STANDALONE_BINARY_NAME=${STANDALONE_BINARY_NAME:-coder}

	if [ "${DRY_RUN-}" ]; then
		echoh "Running with --dry-run; the following are the commands that would be run if this were a real installation:"
		echoh
	fi

	if ! has_standalone; then
		echoerr "There is no binary for $OS-$ARCH"
		exit 1
	fi

	install_standalone
}

parse_arg() {
	case "$1" in
	*=*)
		# Remove everything after first equal sign.
		opt="${1%%=*}"
		# Remove everything before first equal sign.
		optarg="${1#*=}"
		if [ ! "$optarg" ]; then
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
		echoerr "$1 requires an argument"
		echoerr "Run with --help to see usage."
		exit 1
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

install_standalone() {
	echoh "Installing coder-$OS-$ARCH {{ .Version }} from $ORIGIN."
	echoh

	BINARY_FILE="$CACHE_DIR/coder-${OS}-${ARCH}-{{ .Version }}"

	fetch "$ORIGIN/bin/coder-${OS}-${ARCH}" "$BINARY_FILE"

	# -w only works if the directory exists so try creating it first. If this
	# fails we can ignore the error as the -w check will then swap us to sudo.
	sh_c mkdir -p "$STANDALONE_INSTALL_PREFIX" 2>/dev/null || true

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
	"$sh_c" cp "$BINARY_FILE" "$STANDALONE_BINARY_LOCATION"
	"$sh_c" chmod +x "$STANDALONE_BINARY_LOCATION"

	echo_standalone_postinstall
}

# Determine if we have standalone releases on GitHub for the system's arch.
has_standalone() {
	case $ARCH in
	amd64) return 0 ;;
	arm64) return 0 ;;
	armv7)
		[ "$(distro)" = "linux" ]
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
		echo "$XDG_CACHE_HOME/coder/local_downloads"
	elif [ "${HOME-}" ]; then
		echo "$HOME/.cache/coder/local_downloads"
	else
		echo "/tmp/coder-cache/local_downloads"
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
