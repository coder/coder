{
  description = "Development environments on your infrastructure";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    nixpkgs-pinned.url = "github:nixos/nixpkgs/5deee6281831847857720668867729617629ef1f";
    flake-utils.url = "github:numtide/flake-utils";
    pnpm2nix = {
      url = "github:nzbr/pnpm2nix-nzbr";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };
    drpc = {
      url = "github:storj/drpc/v0.0.34";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };
  };

  outputs = { self, nixpkgs, nixpkgs-pinned, flake-utils, drpc, pnpm2nix }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          # Workaround for: terraform has an unfree license (‘bsl11’), refusing to evaluate.
          config.allowUnfree = true;
        };

        # pinnedPkgs is used to pin packages that need to stay in sync with CI.
        # Everything else uses unstable.
        pinnedPkgs = import nixpkgs-pinned {
          inherit system;
        };

        nodejs = pkgs.nodejs-18_x;
        # Check in https://search.nixos.org/packages to find new packages.
        # Use `nix --extra-experimental-features nix-command --extra-experimental-features flakes flake update`
        # to update the lock file if packages are out-of-date.

        # From https://nixos.wiki/wiki/Google_Cloud_SDK
        gdk = pkgs.google-cloud-sdk.withExtraComponents ([ pkgs.google-cloud-sdk.components.gke-gcloud-auth-plugin ]);

        proto_gen_go_1_30 = pkgs.buildGoModule rec {
          name = "protoc-gen-go";
          owner = "protocolbuffers";
          repo = "protobuf-go";
          rev = "v1.30.0"; 
          src = pkgs.fetchFromGitHub {
            owner = "protocolbuffers";
            repo = "protobuf-go";
            rev = rev;
            # Updated with ./scripts/update-flake.sh`.
            sha256 = "sha256-GTZQ40uoi62Im2F4YvlZWiSNNJ4fEAkRojYa0EYz9HU=";
          };
          subPackages = [ "cmd/protoc-gen-go" ];
          vendorHash = null;
          proxyVendor = true;
          preBuild = ''
            export GOPROXY=https://proxy.golang.org,direct
            go mod download
          '';
        };

        # The minimal set of packages to build Coder.
        devShellPackages = with pkgs; [
          # google-chrome is not available on OSX and aarch64 linux
          (if pkgs.stdenv.hostPlatform.isDarwin || pkgs.stdenv.hostPlatform.isAarch64 then null else google-chrome)
          # strace is not available on OSX
          (if pkgs.stdenv.hostPlatform.isDarwin then null else strace)
          bat
          cairo
          curl
          delve
          drpc.defaultPackage.${system}
          gcc
          gdk
          getopt
          gh
          git
          gnumake
          gnused
          go_1_22
          go-migrate
          (pinnedPkgs.golangci-lint)
          gopls
          gotestsum
          jq
          kubectl
          kubectx
          kubernetes-helm
          less
          mockgen
          nfpm
          nodejs
          pnpm
          openssh
          openssl
          pango
          pixman
          pkg-config
          playwright-driver.browsers
          postgresql_16
          protobuf
          proto_gen_go_1_30
          ripgrep
          # This doesn't build on latest nixpkgs (July 10 2024)
          (pinnedPkgs.sapling)
          shellcheck
          (pinnedPkgs.shfmt)
          sqlc
          terraform
          typos
          # Needed for many LD system libs!
          util-linux
          vim
          wget
          yq-go
          zip
          zsh
          zstd
        ];

        # buildSite packages the site directory.
        buildSite = pnpm2nix.packages.${system}.mkPnpmPackage {
          src = ./site/.;
          # Required for the `canvas` package!
          extraBuildInputs = with pkgs; [ pkgs.cairo pkgs.pango pkgs.pixman ];
          installInPlace = true;
          distDir = "out";
        };

        version = "v0.0.0-nix-${self.shortRev or self.dirtyShortRev}";

        # To make faster subsequent builds, you could extract the `.zst`
        # slim bundle into it's own derivation.
        buildFat = osArch:
          pkgs.buildGo122Module {
            name = "coder-${osArch}";
            # Updated with ./scripts/update-flake.sh`.
            # This should be updated whenever go.mod changes!
            vendorHash = "sha256-kPXRp7l05iJd4IdvQeOFOgg2UNzBcloy3tA9Meep9VI=";
            proxyVendor = true;
            src = ./.;
            nativeBuildInputs = with pkgs; [ getopt openssl zstd ];
            preBuild = ''
              # Replaces /usr/bin/env with an absolute path to the interpreter.
              patchShebangs ./scripts
            '';
            buildPhase = ''
              runHook preBuild

              # Unpack the site contents.
              mkdir -p ./site/out
              cp -r ${buildSite.out}/* ./site/out

              # Build and copy the binary!
              export CODER_FORCE_VERSION=${version}
              make -j build/coder_${osArch}
            '';
            installPhase = ''
              mkdir -p $out/bin
              cp -r ./build/coder_${osArch} $out/bin/coder
            '';
          };
      in
      {
        devShell = pkgs.mkShell {
          buildInputs = devShellPackages;
          shellHook = ''
            export PLAYWRIGHT_BROWSERS_PATH=${pkgs.playwright-driver.browsers}
            export PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=true
          '';
        };
        packages = {
          proto_gen_go = proto_gen_go_1_30;  
          all = pkgs.buildEnv {
            name = "all-packages";
            paths = devShellPackages;
          };
          site = buildSite;

          # Copying `OS_ARCHES` from the Makefile.
          linux_amd64 = buildFat "linux_amd64";
          linux_arm64 = buildFat "linux_arm64";
          darwin_amd64 = buildFat "darwin_amd64";
          darwin_arm64 = buildFat "darwin_arm64";
          windows_amd64 = buildFat "windows_amd64.exe";
          windows_arm64 = buildFat "windows_arm64.exe";
        };
      }
    );
}
