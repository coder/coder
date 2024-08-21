# Build stage
FROM nixos/nix:2.19.2 as nix

# enable --experimental-features 'nix-command flakes' globally
# nix does not enable these features by default these are required to run commands like
# nix develop -c 'some command' or to use falke.nix
RUN mkdir -p /etc/nix && \
    echo "experimental-features = nix-command flakes" >> /etc/nix/nix.conf

# Copy Nix flake and install dependencies
COPY flake.* /app/
RUN nix profile install "/app#all" --priority 4 && \
    rm -rf /app && \
    nix-collect-garbage -d

# Final image
FROM codercom/enterprise-base:latest as final

# Set the non-root user
USER root

# Copy the Nix related files into the Docker image
COPY --from=nix --chown=coder:coder /nix /nix
COPY --from=nix /etc/nix /etc/nix
COPY --from=nix --chown=coder:coder /root/.nix-profile /home/coder/.nix-profile
COPY --from=nix /etc/passwd /etc/passwd.nix
COPY --from=nix /etc/group /etc/group.nix

# Merge the passwd and group files
# We need all nix users and groups to be available in the final image
RUN cat /etc/passwd.nix >> /etc/passwd && \
    cat /etc/group.nix >> /etc/group && \
    rm /etc/passwd.nix /etc/group.nix

# Set environment variables and PATH
ENV PATH=/home/coder/.nix-profile/bin:/nix/var/nix/profiles/default/bin:/nix/var/nix/profiles/default/sbin:$PATH \
    GOPRIVATE="coder.com,cdr.dev,go.coder.com,github.com/cdr,github.com/coder" \
    NODE_OPTIONS="--max-old-space-size=8192"

# Set the user to 'coder'
USER coder
WORKDIR /home/coder
