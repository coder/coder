# PostgreSQL Binary Mirror for embedded-postgres
# This image hosts PostgreSQL binaries in the same format as Maven repositories
# to eliminate external dependencies in Coder tests.
#
# Build process:
# 1. Run ./download-postgres-binaries.sh to populate files/ directory
# 2. docker build -t postgres-binary-mirror .

FROM nginx:1.27.3-alpine@sha256:2140dad235c130ac861018a4e13a6bc8aea3a35f3a40e20c1b060d51a7efd250

# Install curl for health checks
RUN apk add --no-cache curl

# Remove default nginx content
RUN rm -rf /usr/share/nginx/html/*

# Copy pre-downloaded PostgreSQL binaries from files/ directory
COPY files/ /usr/share/nginx/html/

# Copy nginx configuration
COPY nginx.conf /etc/nginx/nginx.conf

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Expose port 8080 (Cloud Run standard)
EXPOSE 8080

# Start nginx (default CMD from base image)
# CMD ["nginx", "-g", "daemon off;"] is inherited