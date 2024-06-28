{
  description = "Development environments on your infrastructure";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    pnpm2nix.url = "github:nzbr/pnpm2nix-nzbr";
    drpc.url = "github:storj/drpc/v0.0.33";
  };

  outputs = { self, nixpkgs, flake-utils, drpc, pnpm2nix }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        # Workaround for: terraform has an unfree license (‘bsl11’), refusing to evaluate.
        pkgs = import nixpkgs { inherit system; config.allowUnfree = true; };
        nodejs = pkgs.nodejs-18_x;
        # Check in https://search.nixos.org/packages to find new packages.
        # Use `nix --extra-experimental-features nix-command --extra-experimental-features flakes flake update`
        # to update the lock file if packages are out-of-date.

        # From https://nixos.wiki/wiki/Google_Cloud_SDK
        gdk = pkgs.google-cloud-sdk.withExtraComponents ([ pkgs.google-cloud-sdk.components.gke-gcloud-auth-plugin ]);

        # The minimal set of packages to build Coder.
        devShellPackages = with pkgs; [
          # google-chrome is not available on OSX
          (if pkgs.stdenv.hostPlatform.isDarwin then null else google-chrome)
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
          golangci-lint
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
          nodejs.pkgs.pnpm
          openssh
          openssl
          pango
          pixman
          pkg-config
          playwright-driver.browsers
          postgresql_16
          protobuf
          protoc-gen-go
          ripgrep
          sapling
          shellcheck
          shfmt
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
            vendorHash = "sha256-e0L6osJwG0EF0M3TefxaAjDvN4jvQHxTGEUEECNO1Vw=";
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
