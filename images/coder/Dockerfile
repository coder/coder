FROM postgres:14.0

COPY coderd /coderd
RUN chmod +x /coderd

# Once `provisionerd` is available, we'll also need that binary
# COPY bin/provisionerd /provisionerd
# RUN chmod +x /provisionerd

ENTRYPOINT ["/coderd"]
CMD ["--address", "127.0.0.1:8000"]
