# This image is used to run scaletest jobs and, although it is inside
# the template directory, it is built separately and pushed to
# gcr.io/coder-dev-1/scaletest-runner:latest.
#
# Future improvements will include versioning and including the version
# in the template push.

FROM codercom/enterprise-base:ubuntu

ARG DEBIAN_FRONTEND=noninteractive

USER root

# TODO(mafredri): Remove unneeded dependencies once we have a clear idea of what's needed.
RUN wget --quiet -O /tmp/terraform.zip https://releases.hashicorp.com/terraform/1.5.7/terraform_1.5.7_linux_amd64.zip \
	&& unzip /tmp/terraform.zip -d /usr/local/bin \
	&& rm /tmp/terraform.zip \
	&& terraform --version

RUN wget --quiet -O /tmp/envsubst "https://github.com/a8m/envsubst/releases/download/v1.2.0/envsubst-$(uname -s)-$(uname -m)" \
	&& chmod +x /tmp/envsubst \
	&& mv /tmp/envsubst /usr/local/bin

RUN echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list \
	&& curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key --keyring /usr/share/keyrings/cloud.google.gpg add - \
	&& apt-get update \
	&& apt-get install --yes \
	google-cloud-cli \
	jq \
	kubectl \
	zstd \
	&& gcloud --version \
	&& kubectl version --client \
	&& rm -rf /var/lib/apt/lists/*

USER coder
