FROM ubuntu

RUN apt-get update \
	&& apt-get install -y \
	curl \
	git \
	golang \
	sudo \
	vim \
	wget \
	npm \
	&& rm -rf /var/lib/apt/lists/*

ARG USER=coder
RUN useradd --groups sudo --no-create-home --shell /bin/bash ${USER} \
	&& echo "${USER} ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/${USER} \
	&& chmod 0440 /etc/sudoers.d/${USER}
RUN curl -fL https://install-cli.jfrog.io | sh
RUN chmod 755 $(which jf)
USER ${USER}
WORKDIR /home/${USER}
