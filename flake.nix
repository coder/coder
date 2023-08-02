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
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        formatter = pkgs.nixpkgs-fmt;
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            bash
            bat
            cairo
            drpc.defaultPackage.${system}
            exa
            getopt
            git
            go-migrate
            go_1_20
            golangci-lint
            gopls
            gotestsum
            jq
            kubernetes-helm
            mockgen
            nfpm
            nodePackages.pnpm
            nodePackages.typescript
            nodePackages.typescript-language-server
            nodejs
            openssh
            openssl
            pango
            pixman
            pkg-config
            postgresql
            protoc-gen-go
            ripgrep
            shellcheck
            shfmt
            sqlc
            terraform
            typos
            yq
            zip
            zstd
          ];
        };
      }
    );
}
