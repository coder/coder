#!/usr/bin/env bash

set -euo pipefail

BOLD='\033[0;1m'

printf "%sInstalling filebrowser\n\n" "${BOLD}"

# Check if filebrowser is installed.
if ! command -v filebrowser &>/dev/null; then
	curl -fsSL https://raw.githubusercontent.com/filebrowser/get/master/get.sh | bash
fi

printf "🥳 Installation complete!\n\n"

# Create run script.
cat >/usr/local/bin/filebrowser-entrypoint <<EOF
#!/bin/bash

printf "🛠️ Configuring filebrowser\n\n"

AUTH="${AUTH}"
PORT="${PORT}"
FOLDER="$(pwd)"
LOG_PATH=/tmp/filebrowser.log
export FB_DATABASE="/tmp/filebrowser.db"

# Check if filebrowser db exists.
if [[ ! -f "\${FB_DATABASE}" ]]; then
	filebrowser config init
	if [[ "\$AUTH" == "password" ]]; then
		filebrowser users add admin admin --perm.admin=true --viewMode=mosaic
	fi
fi

# Configure filebrowser.
if [[ "\$AUTH" == "none" ]]; then
	filebrowser config set --port="\${PORT}" --auth.method=noauth --root="\${FOLDER}"
else
	filebrowser config set --port="\${PORT}" --auth.method=json --root="\${FOLDER}"
fi

set -euo pipefail

printf "👷 Starting filebrowser...\n\n"
printf "📂 Serving \${FOLDER} at http://localhost:\${PORT}\n\n"

filebrowser >>\${LOG_PATH} 2>&1 &

printf "📝 Logs at \${LOG_PATH}\n\n"
EOF

chmod +x /usr/local/bin/filebrowser-entrypoint

printf "✅ File Browser installed!\n\n"
printf "🚀 Run 'filebrowser-entrypoint' to start the service\n\n"
