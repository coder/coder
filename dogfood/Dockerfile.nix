FROM nixos/nix:2.21.4

# enable --experimental-features 'nix-command flakes' globally
# nix does not enable these features by default these are required to run commands like
# nix develop -c 'some command' or to use flake.nix
RUN mkdir -p /etc/nix && \
    echo "experimental-features = nix-command flakes" >> /etc/nix/nix.conf

# Add a user `coder` so that you're not developing as the `root` user
RUN useradd coder \
    --create-home \
    --shell=/bin/bash \
    --groups=docker \
    --uid=1000 \
    --user-group && \
    echo "coder ALL=(ALL) NOPASSWD:ALL" >>/etc/sudoers.d/nopasswd

USER coder

# Copy Nix flake and install dependencies
COPY flake.* /tmp/app

RUN nix profile install "/tmp/app#all" --priority 4 && \
    rm -rf /tmp/app && \
    nix-collect-garbage -d

# Set environment variables
ENV GOPRIVATE="coder.com,cdr.dev,go.coder.com,github.com/cdr,github.com/coder" \
    NODE_OPTIONS="--max-old-space-size=8192"
