FROM postgres:14.0

COPY bin/coderd /coderd
RUN chmod +x /coderd

ENTRYPOINT ["/coderd"]
