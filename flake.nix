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
        pkgs = import nixpkgs { inherit system; config.allowUnfree = true; };
        formatter = pkgs.nixpkgs-fmt;
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
          go_1_21
          go-migrate
          golangci-lint
          gopls
          gotestsum
          jq
          kubectl
          kubectx
          kubernetes-helm
          less
          libuuid
          mockgen
          nfpm
          nodejs-18_x
          nodejs-18_x.pkgs.pnpm
          nodejs-18_x.pkgs.prettier
          nodejs-18_x.pkgs.typescript
          nodejs-18_x.pkgs.typescript-language-server
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

        allPackages = pkgs.buildEnv {
          name = "all-packages";
          paths = devShellPackages;
        };
      in
      {
        defaultPackage = formatter;
        devShell = pkgs.mkShell { buildInputs = devShellPackages; };
        packages.all = allPackages;
      }
    );
}
