FROM alpine

ADD ./dist/coder-linux_linux_amd64_v1/coder /opt/coder

ENTRYPOINT [ "/opt/coder", "server" ]
