# docker-compose/setup.Dockerfile
FROM golang:1.25
RUN apt-get update && apt-get install -y --no-install-recommends \
	jq curl unzip \
	&& rm -rf /var/lib/apt/lists/*
# Install Terraform
RUN curl -fsSL https://releases.hashicorp.com/terraform/1.11.2/terraform_1.11.2_linux_amd64.zip \
	-o /tmp/tf.zip && unzip /tmp/tf.zip -d /usr/local/bin && rm /tmp/tf.zip
WORKDIR /app
