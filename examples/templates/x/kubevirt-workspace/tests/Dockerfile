FROM busybox:1.37.0@sha256:66a6306db78bf2dbf3487f293aa8d6990d8e506fdffab9cc43fe422becf886e4
ARG BUILD_MARKER
RUN test -n "$BUILD_MARKER" && printf '%s\n' "$BUILD_MARKER" > /image-marker
CMD ["httpd", "-f", "-p", "8080", "-h", "/workspace"]
