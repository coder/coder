#!/bin/sh

install_devcontainer_cli() {
	# Install @devcontainers/cli from a pinned commit hash (Scorecard-approved) by cloning and building.
	set -e
	SHA="2d577908f9357dd79d309b84e73d55daff828066" # devcontainers/cli commit
	PREFIX="/opt/devcontainers-cli-src"

	if [ ! -d "$PREFIX" ]; then
		git clone https://github.com/devcontainers/cli.git "$PREFIX"
	fi
	cd "$PREFIX"
	git fetch --all --tags --prune
	git checkout --force "$SHA"

	# Build the CLI (produces dist/ consumed by devcontainer.js)
	npm ci
	npm run compile-prod

	# Wrapper to invoke the built CLI
	cat >/usr/local/bin/devcontainer <<'EOF'
#!/usr/bin/env bash
exec node /opt/devcontainers-cli-src/devcontainer.js "$@"
EOF
	chmod +x /usr/local/bin/devcontainer
}

install_ssh_config() {
	echo "🔑 Installing SSH configuration..."
	rsync -a /mnt/home/coder/.ssh/ ~/.ssh/
	chmod 0700 ~/.ssh
}

install_git_config() {
	echo "📂 Installing Git configuration..."
	if [ -f /mnt/home/coder/git/config ]; then
		rsync -a /mnt/home/coder/git/ ~/.config/git/
	elif [ -d /mnt/home/coder/.gitconfig ]; then
		rsync -a /mnt/home/coder/.gitconfig ~/.gitconfig
	else
		echo "⚠️ Git configuration directory not found."
	fi
}

install_dotfiles() {
	if [ ! -d /mnt/home/coder/.config/coderv2/dotfiles ]; then
		echo "⚠️ Dotfiles directory not found."
		return
	fi

	cd /mnt/home/coder/.config/coderv2/dotfiles || return
	for script in install.sh install bootstrap.sh bootstrap script/bootstrap setup.sh setup script/setup; do
		if [ -x $script ]; then
			echo "📦 Installing dotfiles..."
			./$script || {
				echo "❌ Error running $script. Please check the script for issues."
				return
			}
			echo "✅ Dotfiles installed successfully."
			return
		fi
	done
	echo "⚠️ No install script found in dotfiles directory."
}

personalize() {
	# Allow script to continue as Coder dogfood utilizes a hack to
	# synchronize startup script execution.
	touch /tmp/.coder-startup-script.done

	if [ -x /mnt/home/coder/personalize ]; then
		echo "🎨 Personalizing environment..."
		/mnt/home/coder/personalize
	fi
}

install_devcontainer_cli
install_ssh_config
install_dotfiles
personalize
