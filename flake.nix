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
        formatter = pkgs.nixpkgs-fmt;
        devShellPackages = with pkgs; [
          bat
          cairo
          curl
          docker
          drpc.defaultPackage.${system}
          exa
          getopt
          git
          go_1_20
          go-migrate
          golangci-lint
          gopls
          gotestsum
          jq
          kubernetes-helm
          mockgen
          nfpm
          nix
          nodejs
          nodePackages.pnpm
          nodePackages.typescript
          nodePackages.typescript-language-server
          openssh
          openssl
          pango
          pixman
          pkg-config
          postgresql
          protoc-gen-go
          ripgrep
          screen
          shellcheck
          shfmt
          sqlc
          terraform
          typos
          yq
          zip
          zstd
        ];
        baseImage = pkgs.dockerTools.pullImage {
          imageName = "ubuntu";
          imageDigest = "sha256:7a520eeb6c18bc6d32a21bb7edcf673a7830813c169645d51c949cecb62387d0";
          sha256 = "090zricz7n1kbphd7gwhvavj7m1j7bhh4aq3c3mrik5q8pxh4j58";
          finalImageName = "ubuntu";
          finalImageTag = "lunar";
        };
        dockerImage = pkgs.dockerTools.buildLayeredImage {
          name = "dev-environment";
          fromImage = baseImage;
          contents = with pkgs; [
            terraform
          ];
          # extraCommands = ''
          # mv bin nixbin
          # ln -s usr/bin bin
          # '';
          config = {
            # Env = [ "PATH=${pkgs.lib.makeBinPath devShellPackages}:$PATH" ];
            Entrypoint = [ "/bin/bash" ];
          };
        };
      in
      {
        packages = {
          devEnvironmentDocker = dockerImage;
          # other packages you want to define for this system
        };
        defaultPackage = formatter; # or replace it with your desired default package.
        devShell = pkgs.mkShell { buildInputs = devShellPackages; };
      }
    );
}
