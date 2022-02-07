FROM registry.access.redhat.com/ubi8/ubi:latest

COPY coderd /coder/coderd
RUN chmod +x /coder/coderd

COPY run.sh /coder/run.sh
RUN chmod +x /coder/run.sh

# Once `provisionerd` is available, we'll also need that binary
# COPY bin/provisionerd /provisionerd
# RUN chmod +x /provisionerd

ENTRYPOINT ["/coder/run.sh"]
