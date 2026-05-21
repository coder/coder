#!/usr/bin/env bash

set -euo pipefail

BOLD='\033[0;1m'

printf "%sInstalling filebrowser\n\n" "${BOLD}"

# Check if filebrowser is installed.
if ! command -v filebrowser &>/dev/null; then
	VERSION="v2.42.1"
	EXPECTED_HASH="7d83c0f077df10a8ec9bfd9bf6e745da5d172c3c768a322b0e50583a6bc1d3cc"

	curl -fsSL "https://github.com/filebrowser/filebrowser/releases/download/${VERSION}/linux-amd64-filebrowser.tar.gz" -o /tmp/filebrowser.tar.gz
	echo "${EXPECTED_HASH} /tmp/filebrowser.tar.gz" | sha256sum -c
	tar -xzf /tmp/filebrowser.tar.gz -C /tmp
	sudo mv /tmp/filebrowser /usr/local/bin/
	sudo chmod +x /usr/local/bin/filebrowser
	rm /tmp/filebrowser.tar.gz
fi

# Create entrypoint.
cat >/usr/local/bin/filebrowser-entrypoint <<EOF
#!/usr/bin/env bash

PORT="${PORT}"
FOLDER="${FOLDER:-}"
FOLDER="\${FOLDER:-\$(pwd)}"
BASEURL="${BASEURL:-}"
LOG_PATH=/tmp/filebrowser.log
export FB_DATABASE="\${HOME}/.filebrowser.db"

printf "🛠️ Configuring filebrowser\n\n"

# Check if filebrowser db exists.
if [[ ! -f "\${FB_DATABASE}" ]]; then
	filebrowser config init >>\${LOG_PATH} 2>&1
	filebrowser users add admin "" --perm.admin=true --viewMode=mosaic >>\${LOG_PATH} 2>&1
fi

filebrowser config set --baseurl=\${BASEURL} --port=\${PORT} --auth.method=noauth --root=\${FOLDER} >>\${LOG_PATH} 2>&1

printf "👷 Starting filebrowser...\n\n"

printf "📂 Serving \${FOLDER} at http://localhost:\${PORT}\n\n"

filebrowser >>\${LOG_PATH} 2>&1 &

printf "📝 Logs at \${LOG_PATH}\n\n"
EOF

chmod +x /usr/local/bin/filebrowser-entrypoint

printf "🥳 Installation complete!\n\n"
