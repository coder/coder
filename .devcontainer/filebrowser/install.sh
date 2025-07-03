#!/usr/bin/env bash

set -euo pipefail

BOLD='\033[0;1m'

printf "%sInstalling filebrowser\n\n" "${BOLD}"

# Check if filebrowser is installed.
if ! command -v filebrowser &>/dev/null; then
	curl -fsSL https://raw.githubusercontent.com/filebrowser/get/master/get.sh | bash
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
