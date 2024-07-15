FROM nixos/nix:2.21.4

# enable --experimental-features 'nix-command flakes' globally
# nix does not enable these features by default these are required to run commands like
# nix develop -c 'some command' or to use flake.nix
RUN mkdir -p /etc/nix && \
    echo "experimental-features = nix-command flakes" >> /etc/nix/nix.conf

# Copy Nix flake and install dependencies
COPY flake.* /tmp/

RUN nix profile install "/tmp#all" --impure --priority 4 && nix-collect-garbage -d

# Set environment variables
ENV GOPRIVATE="coder.com,cdr.dev,go.coder.com,github.com/cdr,github.com/coder" \
    NODE_OPTIONS="--max-old-space-size=8192"
