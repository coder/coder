FROM postgres:14.0

COPY bin/coderd /coderd
RUN chmod +x /coderd

# Once `provisionerd` is available, we'll also need that binary
# COPY bin/provisionerd /provisionerd
# RUN chmod +x /provisionerd

# TODO: Do we need a specific port for coderd? Default is 3000
ENTRYPOINT ["/coderd"]
