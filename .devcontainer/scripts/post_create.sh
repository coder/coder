#!/bin/sh

install_devcontainer_cli() {
	set -e
	echo "🔧 Installing DevContainer CLI..."
	cd "$(dirname "$0")/../tools/devcontainer-cli"
	npm ci --omit=dev
	ln -sf "$(pwd)/node_modules/.bin/devcontainer" "$(npm config get prefix)/bin/devcontainer"
}

install_ssh_config() {
	echo "🔑 Installing SSH configuration..."
	if [ -d /mnt/home/coder/.ssh ]; then
		rsync -a /mnt/home/coder/.ssh/ ~/.ssh/
		chmod 0700 ~/.ssh
	else
		echo "⚠️ SSH directory not found."
	fi
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
