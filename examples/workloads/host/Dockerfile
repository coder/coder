FROM docker:27.5.1-dind

RUN apk add --no-cache bash curl jq openssl \
    && mkdir -p /opt/workloads/project
COPY start-host.sh prepare-host.sh /opt/workloads/
ENV DOCKER_TLS_CERTDIR=""
ENTRYPOINT ["bash", "/opt/workloads/start-host.sh"]
