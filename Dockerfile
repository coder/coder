FROM alpine

ADD coder /opt/coder

ENTRYPOINT [ "/opt/coder", "server" ]
