FROM ubuntu@sha256:99c35190e22d294cdace2783ac55effc69d32896daaa265f0bbedbcde4fbe3e5

RUN apt-get update \
	&& apt-get install -y \
	curl \
	git \
	python3-pip \
	sudo \
	vim \
	wget \
	npm \
	&& rm -rf /var/lib/apt/lists/*

ARG GO_VERSION=1.20.8
RUN mkdir --parents /usr/local/go && curl --silent --show-error --location \
	"https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /usr/local/go.tar.gz && \
	tar --extract --gzip --directory=/usr/local/go --file=/usr/local/go.tar.gz --strip-components=1

ENV PATH=$PATH:/usr/local/go/bin

ARG USER=coder
RUN useradd --groups sudo --no-create-home --shell /bin/bash ${USER} \
	&& echo "${USER} ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/${USER} \
	&& chmod 0440 /etc/sudoers.d/${USER}
RUN curl -fL https://install-cli.jfrog.io | sh
RUN chmod 755 $(which jf)
USER ${USER}
WORKDIR /home/${USER}
