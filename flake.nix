{
  description = "Development environments on your infrastructure";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-24.11";
    nixpkgs-unstable.url = "github:nixos/nixpkgs/nixos-unstable";
    nixpkgs-pinned.url = "github:nixos/nixpkgs/5deee6281831847857720668867729617629ef1f";
    flake-utils.url = "github:numtide/flake-utils";
    pnpm2nix = {
      url = "github:ThomasK33/pnpm2nix-nzbr";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };
    drpc = {
      url = "github:storj/drpc/v0.0.34";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      nixpkgs-pinned,
      nixpkgs-unstable,
      flake-utils,
      drpc,
      pnpm2nix,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          # Workaround for: google-chrome has an unfree license (‘unfree’), refusing to evaluate.
          config.allowUnfree = true;
        };

        # pinnedPkgs is used to pin packages that need to stay in sync with CI.
        # Everything else uses unstable.
        pinnedPkgs = import nixpkgs-pinned {
          inherit system;
        };

        unstablePkgs = import nixpkgs-unstable {
          inherit system;

          # Workaround for: terraform has an unfree license (‘bsl11’), refusing to evaluate.
          config.allowUnfreePredicate =
            pkg:
            builtins.elem (pkgs.lib.getName pkg) [
              "terraform"
            ];
        };

        formatter = pkgs.nixfmt-rfc-style;

        nodejs = pkgs.nodejs_20;
        pnpm = pkgs.pnpm_9.override {
          inherit nodejs; # Ensure it points to the above nodejs version
        };

        # Check in https://search.nixos.org/packages to find new packages.
        # Use `nix --extra-experimental-features nix-command --extra-experimental-features flakes flake update`
        # to update the lock file if packages are out-of-date.

        # From https://nixos.wiki/wiki/Google_Cloud_SDK
        gdk = pkgs.google-cloud-sdk.withExtraComponents [
          pkgs.google-cloud-sdk.components.gke-gcloud-auth-plugin
        ];

        proto_gen_go_1_30 = pkgs.buildGoModule rec {
          name = "protoc-gen-go";
          owner = "protocolbuffers";
          repo = "protobuf-go";
          rev = "v1.30.0";
          src = pkgs.fetchFromGitHub {
            inherit owner repo rev;
            # Updated with ./scripts/update-flake.sh`.
            sha256 = "sha256-GTZQ40uoi62Im2F4YvlZWiSNNJ4fEAkRojYa0EYz9HU=";
          };
          subPackages = [ "cmd/protoc-gen-go" ];
          vendorHash = null;
        };

        # Packages required to build the frontend
        frontendPackages =
          with pkgs;
          [
            cairo
            pango
            pixman
            libpng
            libjpeg
            giflib
            librsvg
            python312Packages.setuptools # Needed for node-gyp
          ]
          ++ (lib.optionals stdenv.targetPlatform.isDarwin [
            darwin.apple_sdk.frameworks.Foundation
            xcbuild
          ]);

        # The minimal set of packages to build Coder.
        devShellPackages =
          with pkgs;
          [
            # google-chrome is not available on aarch64 linux
            (lib.optionalDrvAttr (!stdenv.isLinux || !stdenv.isAarch64) google-chrome)
            # strace is not available on OSX
            (lib.optionalDrvAttr (!pkgs.stdenv.isDarwin) strace)
            bat
            cairo
            curl
            cosign
            delve
            dive
            drpc.defaultPackage.${system}
            formatter
            fzf
            gawk
            gcc13
            gdk
            getopt
            gh
            git
            (lib.optionalDrvAttr stdenv.isLinux glibcLocales)
            gnumake
            gnused
            gnugrep
            gnutar
            go_1_22
            go-migrate
            (pinnedPkgs.golangci-lint)
            gopls
            gotestsum
            hadolint
            jq
            kubectl
            kubectx
            kubernetes-helm
            lazygit
            less
            mockgen
            moreutils
            neovim
            nfpm
            nix-prefetch-git
            nodejs
            openssh
            openssl
            pango
            pixman
            pkg-config
            playwright-driver.browsers
            pnpm
            postgresql_16
            proto_gen_go_1_30
            protobuf_23
            ripgrep
            shellcheck
            (pinnedPkgs.shfmt)
            sqlc
            syft
            unstablePkgs.terraform
            typos
            which
            # Needed for many LD system libs!
            (lib.optional stdenv.isLinux util-linux)
            vim
            wget
            yq-go
            zip
            zsh
            zstd
          ]
          ++ frontendPackages;

        docker = pkgs.callPackage ./nix/docker.nix { };

        # buildSite packages the site directory.
        buildSite = pnpm2nix.packages.${system}.mkPnpmPackage {
          inherit nodejs pnpm;

          src = ./site/.;
          # Required for the `canvas` package!
          extraBuildInputs = frontendPackages;
          installInPlace = true;
          distDir = "out";
        };

        version = "v0.0.0-nix-${self.shortRev or self.dirtyShortRev}";

        # To make faster subsequent builds, you could extract the `.zst`
        # slim bundle into it's own derivation.
        buildFat =
          osArch:
          pkgs.buildGo122Module {
            name = "coder-${osArch}";
            # Updated with ./scripts/update-flake.sh`.
            # This should be updated whenever go.mod changes!
            vendorHash = "sha256-6sdvX0Wglj0CZiig2VD45JzuTcxwg7yrGoPPQUYvuqU=";
            proxyVendor = true;
            src = ./.;
            nativeBuildInputs = with pkgs; [
              getopt
              openssl
              zstd
            ];
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
      # "Keep in mind that you need to use the same version of playwright in your node playwright project as in your nixpkgs, or else playwright will try to use browsers versions that aren't installed!"
      # - https://nixos.wiki/wiki/Playwright
      assert pkgs.lib.assertMsg
        (
          (pkgs.lib.importJSON ./site/package.json).devDependencies."@playwright/test"
          == pkgs.playwright-driver.version
        )
        "There is a mismatch between the playwright versions in the ./nix.flake and the ./site/package.json file. Please make sure that they use the exact same version.";
      rec {
        inherit formatter;

        devShells = {
          default = pkgs.mkShell {
            buildInputs = devShellPackages;

            PLAYWRIGHT_BROWSERS_PATH = pkgs.playwright-driver.browsers;
            PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS = true;

            LOCALE_ARCHIVE =
              with pkgs;
              lib.optionalDrvAttr stdenv.isLinux "${glibcLocales}/lib/locale/locale-archive";

            NODE_OPTIONS = "--max-old-space-size=8192";
            GOPRIVATE = "coder.com,cdr.dev,go.coder.com,github.com/cdr,github.com/coder";
          };
        };

        packages =
          {
            default = packages.${system};

            proto_gen_go = proto_gen_go_1_30;
            site = buildSite;

            # Copying `OS_ARCHES` from the Makefile.
            x86_64-linux = buildFat "linux_amd64";
            aarch64-linux = buildFat "linux_arm64";
            x86_64-darwin = buildFat "darwin_amd64";
            aarch64-darwin = buildFat "darwin_arm64";
            x86_64-windows = buildFat "windows_amd64.exe";
            aarch64-windows = buildFat "windows_arm64.exe";
          }
          // (pkgs.lib.optionalAttrs pkgs.stdenv.isLinux {
            dev_image = docker.buildNixShellImage rec {
              name = "codercom/oss-dogfood-nix";
              tag = "latest-${system}";

              # (ThomasK33): Workaround for images with too many layers (>64 layers) causing sysbox
              # to have issues on dogfood envs.
              maxLayers = 32;

              uname = "coder";
              homeDirectory = "/home/${uname}";
              releaseName = version;

              drv = devShells.default.overrideAttrs (oldAttrs: {
                buildInputs =
                  (with pkgs; [
                    coreutils
                    nix.out
                    curl.bin # Ensure the actual curl binary is included in the PATH
                    glibc.bin # Ensure the glibc binaries are included in the PATH
                    jq.bin
                    binutils # ld and strings
                    filebrowser # Ensure that we're not redownloading filebrowser on each launch
                    systemd.out
                    service-wrapper
                    docker_26
                    shadow.out
                    su
                    ncurses.out # clear
                    unzip
                    zip
                    gzip
                    procps # free
                  ])
                  ++ oldAttrs.buildInputs;
              });
            };
          });
      }
    );
}
