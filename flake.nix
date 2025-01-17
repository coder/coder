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

        nodejs = pkgs.nodejs_20;
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
        };

        # The minimal set of packages to build Coder.
        devShellPackages = with pkgs; [
          # google-chrome is not available on aarch64 linux
          (lib.optionalDrvAttr ( !stdenv.isLinux || !stdenv.isAarch64 ) google-chrome)
          # strace is not available on OSX
          (lib.optionalDrvAttr ( !pkgs.stdenv.isDarwin ) strace)
          bat
          cairo
          curl
          delve
          drpc.defaultPackage.${system}
          fzf
          gcc
          gdk
          getopt
          gh
          git
          (lib.optionalDrvAttr stdenv.isLinux glibcLocales)
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
          lazygit
          less
          mockgen
          moreutils
          nix-prefetch-git
          nfpm
          nodejs
          neovim
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
          inherit nodejs;

          src = ./site/.;
          # Required for the `canvas` package!
          extraBuildInputs = with pkgs; [
            cairo
            pango
            pixman
            libpng libjpeg giflib librsvg
            python312Packages.setuptools
          ] ++ ( lib.optionals stdenv.targetPlatform.isDarwin [ darwin.apple_sdk.frameworks.Foundation xcbuild ] );
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
            vendorHash = "sha256-ykLZqtALSvDpBc2yEjRGdOyCFNsnLZiGid0d4s27e8Q=";
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
              mkdir -p ./site/out ./site/node_modules/
              cp -r ${buildSite.out}/* ./site/out
              touch ./site/node_modules/.installed

              # Build and copy the binary!
              export CODER_FORCE_VERSION=${version}
              # Flagging 'site/node_modules/.installed' as an old file,
              # as we do not want to trigger codegen during a build.
              make -j -o 'site/node_modules/.installed' build/coder_${osArch}
            '';
            installPhase = ''
              mkdir -p $out/bin
              cp -r ./build/coder_${osArch} $out/bin/coder
            '';
          };
      in
      {
        devShells = {
          default = pkgs.mkShell {
            buildInputs = devShellPackages;
            shellHook = ''
              export PLAYWRIGHT_BROWSERS_PATH=${pkgs.playwright-driver.browsers}
              export PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=true
            '';

            LOCALE_ARCHIVE = with pkgs; lib.optionalDrvAttr stdenv.isLinux "${glibcLocales}/lib/locale/locale-archive";
          };
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
