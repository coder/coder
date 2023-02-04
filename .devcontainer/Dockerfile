FROM ubuntu
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

ENV EDITOR=vim

RUN apt-get update && apt-get upgrade --yes

RUN apt-get install --yes \
	ca-certificates \
	bash-completion \
	build-essential \
	curl \
	cmake \
	direnv \
	emacs-nox \
	gnupg \
	htop \
	jq \
	less \
	lsb-release \
	lsof \
	man-db \
	nano \
	neovim \
	ssl-cert \
	sudo \
	unzip \
	xz-utils \
	zip

# configure locales to UTF8
RUN apt-get install locales && locale-gen en_US.UTF-8
ENV LANG='en_US.UTF-8' LANGUAGE='en_US:en' LC_ALL='en_US.UTF-8'

# configure direnv
RUN direnv hook bash >> $HOME/.bashrc

# install nix
RUN sh <(curl -L https://nixos.org/nix/install) --daemon

RUN mkdir -p $HOME/.config/nix $HOME/.config/nixpkgs \
	&& echo 'sandbox = false' >> $HOME/.config/nix/nix.conf \
	&& echo '{ allowUnfree = true; }' >> $HOME/.config/nixpkgs/config.nix \
	&& echo '. $HOME/.nix-profile/etc/profile.d/nix.sh' >> $HOME/.bashrc


# install docker and configure daemon to use vfs as GitHub codespaces requires vfs
# https://github.com/moby/moby/issues/13742#issuecomment-725197223
RUN mkdir -p /etc/apt/keyrings \
	&& curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
	&& echo \
	"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
	$(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null \
	&& apt-get update \
	&& apt-get install --yes docker-ce docker-ce-cli containerd.io docker-compose-plugin \
	&& mkdir -p /etc/docker \
	&& echo '{"cgroup-parent":"/actions_job","storage-driver":"vfs"}' >> /etc/docker/daemon.json

# install golang and language tooling
ENV GO_VERSION=1.20
ENV GOPATH=$HOME/go-packages
ENV GOROOT=$HOME/go
ENV PATH=$GOROOT/bin:$GOPATH/bin:$PATH
RUN curl -fsSL https://dl.google.com/go/go$GO_VERSION.linux-amd64.tar.gz | tar xzs
RUN echo 'export PATH=$GOPATH/bin:$PATH' >> $HOME/.bashrc

RUN bash -c ". $HOME/.bashrc \
	go install -v golang.org/x/tools/gopls@latest \
	&& go install -v mvdan.cc/sh/v3/cmd/shfmt@latest \
	&& go install -v github.com/mikefarah/yq/v4@v4.30.6 \
	"

# install nodejs
RUN bash -c "$(curl -fsSL https://deb.nodesource.com/setup_14.x)" \
	&& apt-get install -y nodejs

# install zstd
RUN bash -c "$(curl -fsSL https://raw.githubusercontent.com/horta/zstd.install/main/install)"

# install nfpm
RUN echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | sudo tee /etc/apt/sources.list.d/goreleaser.list \
	&& apt update \
	&& apt install nfpm
