{
  description = "Development environments on your infrastructure";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    drpc.url = "github:storj/drpc/v0.0.32";
  };

  outputs = { self, nixpkgs, flake-utils, drpc }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        # Workaround for: terraform has an unfree license (‘bsl11’), refusing to evaluate.
        pkgs = import nixpkgs { inherit system; config.allowUnfree = true; };
        formatter = pkgs.nixpkgs-fmt;
        # Check in https://search.nixos.org/packages to find new packages.
        # Use `nix --extra-experimental-features nix-command --extra-experimental-features flakes flake update`
        # to update the lock file if packages are out-of-date.

        # From https://nixos.wiki/wiki/Google_Cloud_SDK
        gdk = pkgs.google-cloud-sdk.withExtraComponents ([pkgs.google-cloud-sdk.components.gke-gcloud-auth-plugin]);

        devShellPackages = with pkgs; [
          bat
          cairo
          curl
          drpc.defaultPackage.${system}
          gcc
          gdk
          getopt
          git
          gh
          gnumake
          gnused
          go_1_20
          go-migrate
          golangci-lint
          gopls
          gotestsum
          jq
          kubectl
          kubectx
          kubernetes-helm
          less
          # Needed for many LD system libs!
          libuuid
          mockgen
          nfpm
          nodejs
          nodePackages.pnpm
          nodePackages.prettier
          nodePackages.typescript
          nodePackages.typescript-language-server
          openssh
          openssl
          pango
          pixman
          pkg-config
          postgresql_13
          protobuf
          protoc-gen-go
          ripgrep
          sapling
          shellcheck
          shfmt
          sqlc
          # strace is not available on OSX
          (if pkgs.stdenv.hostPlatform.isDarwin then null else strace)
          terraform
          typos
          vim
          wget
          yarn
          yq-go
          zip
          zsh
          zstd
        ];
      in
      {
        defaultPackage = formatter; # or replace it with your desired default package.
        devShell = pkgs.mkShell { buildInputs = devShellPackages; };
      }
    );
}
