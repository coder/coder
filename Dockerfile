FROM alpine

# The coder binary is injected by scripts/build_docker.sh.
ADD coder /opt/coder

ENTRYPOINT [ "/opt/coder", "server" ]
