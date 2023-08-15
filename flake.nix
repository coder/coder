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
          bash
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
          sha256 = "1qa9nq3rir0wnhbs15mwbilzw530x7ih9pq5q1wv3axz44ap6dka";
          finalImageName = "ubuntu";
          finalImageTag = "lunar";
        };
        dockerImage = pkgs.dockerTools.streamLayeredImage {
          name = "dev-environment";
          fromImage = baseImage;
          extraCommands = ''
          touch ./.wh.bin
          ln -s usr/bin bin
          '';

          config = {
            Env = [
              "PATH=${pkgs.lib.makeBinPath devShellPackages}:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"
            ];
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
