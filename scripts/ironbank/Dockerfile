ARG BASE_REGISTRY=registry1.dso.mil
ARG BASE_IMAGE=ironbank/redhat/ubi/ubi8-minimal
ARG BASE_TAG=8.7

FROM ${BASE_REGISTRY}/${BASE_IMAGE}:${BASE_TAG}

SHELL ["/bin/bash", "-c"]

ENV LANG=en_US.UTF-8

RUN microdnf update --assumeyes && \
    microdnf install --assumeyes \
      ca-certificates \
      git \
      gzip \
      shadow-utils \
      tar \
      unzip && \
    microdnf clean all

# Configure the cryptography policy manually. These policies likely
# have no impact, since Go doesn't link against these libraries.
#
# Normally, one uses the update-crypto-policies script to create these
# links, which is included in the crypto-policies-scripts package, but
# that pulls in Python, so we create the links manually here. This
# list of links comes from running strace on the update-crypto-policies
# script (strace update-crypto-policies --set FIPS) in Fedora, since
# RHEL and UBI do not provide an strace package by default.
RUN echo "FIPS" >/etc/crypto-policies/config && \
    cp --force /usr/share/crypto-policies/policies/FIPS.pol /etc/crypto-policies/state/CURRENT.pol && \
    echo "FIPS" >/etc/crypto-policies/state/current && \
    ln --symbolic --force /usr/share/crypto-policies/FIPS/bind.txt /etc/crypto-policies/back-ends/bind.config && \
    ln --symbolic --force /usr/share/crypto-policies/FIPS/gnutls.txt /etc/crypto-policies/back-ends/gnutls.config && \
    ln --symbolic --force /usr/share/crypto-policies/FIPS/java.txt /etc/crypto-policies/back-ends/java.config && \
    ln --symbolic --force /usr/share/crypto-policies/FIPS/krb5.txt /etc/crypto-policies/back-ends/krb5.config && \
    ln --symbolic --force /usr/share/crypto-policies/FIPS/libreswan.txt /etc/crypto-policies/back-ends/libreswan.config && \
    ln --symbolic --force /usr/share/crypto-policies/FIPS/libssh.txt /etc/crypto-policies/back-ends/libssh.config && \
    ln --symbolic --force /usr/share/crypto-policies/FIPS/nss.txt /etc/crypto-policies/back-ends/nss.config && \
    ln --symbolic --force /usr/share/crypto-policies/FIPS/openssh.txt /etc/crypto-policies/back-ends/openssh.config && \
    ln --symbolic --force /usr/share/crypto-policies/FIPS/opensshserver.txt /etc/crypto-policies/back-ends/opensshserver.config && \
    ln --symbolic --force /usr/share/crypto-policies/FIPS/openssl.txt /etc/crypto-policies/back-ends/openssl.config && \
    ln --symbolic --force /usr/share/crypto-policies/FIPS/opensslcnf.txt /etc/crypto-policies/back-ends/opensslcnf.config

# Copy and extract Coder binary from tar file. We have to put this in /opt to
# match the Dockerfile.
ARG CODER_BIN=/opt/coder
ARG CODER_BIN_TAR_GZ=coder.tar.gz
COPY "$CODER_BIN_TAR_GZ" /tmp/coder.tar.gz
RUN mkdir -p /opt && \
		tar -xzvf /tmp/coder.tar.gz --directory /opt --strip-components=1 ./coder && \
    rm /tmp/coder.tar.gz
ENV PATH="/opt:${PATH}"

# Copy and extract Terraform binary from zip file.
ARG TERRAFORM_BIN_DIR=/opt/terraform
ARG TERRAFORM_BIN_ZIP=terraform.zip
COPY "$TERRAFORM_BIN_ZIP" /tmp/terraform.zip
RUN mkdir -p "$TERRAFORM_BIN_DIR" && \
		unzip /tmp/terraform.zip -d "$TERRAFORM_BIN_DIR" && \
	  rm /tmp/terraform.zip
ENV PATH="${TERRAFORM_BIN_DIR}:${PATH}"

# Install the Coder Terraform provider to a well-known location.
ARG TERRAFORM_PLUGINS_DIR=/opt/terraform/plugins
ARG TERRAFORM_CODER_PROVIDER_VERSION
ARG TERRAFORM_CODER_PROVIDER_ZIP=terraform-provider-coder.zip
COPY "$TERRAFORM_CODER_PROVIDER_ZIP" "${TERRAFORM_PLUGINS_DIR}/registry.terraform.io/coder/coder/terraform-provider-coder_${TERRAFORM_CODER_PROVIDER_VERSION}_linux_amd64.zip"

# Configure Terraform to use plugins from this dir.
COPY terraform-filesystem-mirror.tfrc /opt/terraform/config.tfrc
ENV TF_CLI_CONFIG_FILE=/opt/terraform/config.tfrc

# Uninstall the build dependencies.
RUN microdnf remove --assumeyes \
      tar \
      unzip && \
    microdnf clean all

# Transfer ownership of the binaries to the 'coder' user.
RUN useradd coder \
      --create-home \
      --shell=/bin/bash \
      --uid=1000 \
      --user-group && \
    chown --recursive --quiet coder:coder "$CODER_BIN" && \
    chown --recursive --quiet coder:coder "$TERRAFORM_BIN_DIR" && \
    chown --recursive --quiet coder:coder "$TERRAFORM_PLUGINS_DIR" && \
    chmod 0755 /home/coder

USER 1000
ENV HOME /home/coder
ENV USER=coder

ENTRYPOINT [ "/opt/coder", "server" ]
