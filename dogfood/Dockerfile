# Build stage
FROM nixos/nix:2.19.2 as nix

# enable --experimental-features 'nix-command flakes' globally
RUN mkdir -p /etc/nix && echo "experimental-features = nix-command flakes" >> /etc/nix/nix.conf

# Copy the Nix related files into the Docker image
COPY flake.nix /app/flake.nix
COPY flake.lock /app/flake.lock

# Install dependencies from flake and remove the flake
RUN nix profile install /app#all --priority 4 && rm -rf /app

# print all users and groups
RUN cp /etc/passwd /etc/passwd.nix && cp /etc/group /etc/group.nix

# Final image
FROM codercom/enterprise-base:latest as final

USER root

# Copy the Nix related files into the Docker image
COPY --from=nix /nix /nix
COPY --from=nix /etc/nix /etc/nix
COPY --from=nix /root/.nix-profile /root/.nix-profile
COPY --from=nix /root/.nix-defexpr /root/.nix-defexpr
COPY --from=nix /root/.nix-channels /root/.nix-channels

# Merge the passwd and group files
COPY --from=nix /etc/passwd.nix /etc/passwd.nix
COPY --from=nix /etc/group.nix /etc/group.nix
RUN cat /etc/passwd.nix >> /etc/passwd && cat /etc/group.nix >> /etc/group && rm /etc/passwd.nix && rm /etc/group.nix

# Update the PATH to include the Nix stuff
ENV PATH=/root/.nix-profile/bin:/nix/var/nix/profiles/default/bin:/nix/var/nix/profiles/default/sbin:$PATH

# Install npm global packages
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get upgrade -y && \
    npm install -g pnpm playwright@1.36.2 && npx playwright install-deps && npm cache clean --force && \
    rm -rf /var/lib/apt/lists/*

# Set environment variables
ENV GOPRIVATE="coder.com,cdr.dev,go.coder.com,github.com/cdr,github.com/coder"

# Increase memory allocation to NodeJS
ENV NODE_OPTIONS="--max-old-space-size=8192"

USER coder
