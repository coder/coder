{
  description = "Development environments on your infrastructure";

  inputs = {
    nixpkgs-unstable.url = "nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    drpc = {
      url = "github:storj/drpc";
      inputs = {
        nixpkgs.follows = "nixpkgs-unstable";
        flake-utils.follows = "flake-utils";
      };
    };
  };

  outputs = { self, nixpkgs-unstable, flake-utils, drpc }:
    flake-utils.lib.eachDefaultSystem (system:
      with nixpkgs-unstable.legacyPackages.${system}; rec {
        devShell =
          let devtools = {};
        in mkShell {
            buildInputs = [
              drpc.defaultPackage.${system}
            ];
            nativeBuildInputs = [
              go_1_19
              gopls
              nodejs
              ripgrep
              exa
              bat
              typos
              git
              nfpm
              openssl
              protoc-gen-go
              go-migrate
              gotestsum
              goreleaser
              sqlc
              shfmt
              terraform
              shellcheck
              golangci-lint
              yarn
              postgresql
              helm
              jq
              zstd
              zip
              openssh
              nodePackages.typescript
              nodePackages.typescript-language-server
            ];
        };
      });
}
