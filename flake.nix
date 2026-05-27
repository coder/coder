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

        nodejs = unstablePkgs.nodejs_22;
        pnpm = pkgs.pnpm_10.override {
          inherit nodejs; # Ensure it points to the above nodejs version
        };

        mise = pkgs.stdenvNoCC.mkDerivation rec {
          pname = "mise";
          version = "2026.5.12";
          target = {
            x86_64-linux = "linux-x64";
            aarch64-linux = "linux-arm64";
            x86_64-darwin = "macos-x64";
            aarch64-darwin = "macos-arm64";
          }.${system};
          src = pkgs.fetchurl {
            url = "https://github.com/jdx/mise/releases/download/v${version}/mise-v${version}-${target}";
            hash = {
              x86_64-linux = "sha256-ojiXKjFi1xC4WyjDJDculspOS0hsgf54aVAA2fvHfEg=";
              aarch64-linux = "sha256-/S1SJ6itCx41nHBSeoNFqa2nIHf43LtVk3FlPD2VRk8=";
              x86_64-darwin = "sha256-3lfo3IK72ICmnJvIruBrncxXgYSz5c+G/O+AY11qkLQ=";
              aarch64-darwin = "sha256-53cHBUD/4iz4srn4iu2ItGHQiH2UDE8cGpc1lGPN5uE=";
            }.${system};
          };
          dontUnpack = true;
          installPhase = ''
            install -Dm755 "$src" "$out/bin/mise"
          '';
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

        # Custom sqlc build from coder/sqlc fork to fix ambiguous column bug, see:
        # - https://github.com/coder/sqlc/pull/1
        # - https://github.com/sqlc-dev/sqlc/pull/4159
        #
        # To update hashes:
        # 1. Run: `nix --extra-experimental-features 'nix-command flakes' build .#devShells.x86_64-linux.default`
        # 2. Nix will fail with the correct sha256 hash for src
        # 3. Update the sha256 and run again
        # 4. Nix will fail with the correct vendorHash
        # 5. Update the vendorHash
        sqlc-custom = unstablePkgs.buildGo126Module {
          pname = "sqlc";
          version = "coder-fork-337309bfb9524f38466a5090e310040fc7af0203";

          src = pkgs.fetchFromGitHub {
            owner = "coder";
            repo = "sqlc";
            rev = "337309bfb9524f38466a5090e310040fc7af0203";
            sha256 = "sha256-i8hZaaMlNJyW0hUWYcuNqUcwRdQU747055OknZsJ9Es=";
          };

          subPackages = [ "cmd/sqlc" ];
          vendorHash = "sha256-4Cb15MhKyhRvYVKfMqBwuC3WBBIJE6AinJt02+TSMVY=";
        };

        # Keep Terraform aligned with provisioner/terraform/testdata/version.txt
        # so `make gen` remains deterministic in Nix shells.
        terraform_1_15_5 =
          if pkgs.stdenv.isLinux && pkgs.stdenv.hostPlatform.isx86_64 then
            pkgs.runCommand "terraform-1.15.5" {
              nativeBuildInputs = [ pkgs.unzip ];
              src = pkgs.fetchurl {
                url = "https://releases.hashicorp.com/terraform/1.15.5/terraform_1.15.5_linux_amd64.zip";
                hash = "sha256-cCshNq9nKMj/A3+EPdLbzit62IeGtzgdHXKu+iUPYBw=";
              };
            } ''
              mkdir -p "$out/bin"
              unzip -p "$src" terraform > "$out/bin/terraform"
              chmod +x "$out/bin/terraform"
            ''
          else
            unstablePkgs.terraform;

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
            darwin.apple_sdk_12_3.frameworks.Foundation
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
            git-lfs
            (lib.optionalDrvAttr stdenv.isLinux glibcLocales)
            gnumake
            gnused
            gnugrep
            gnutar
            unstablePkgs.go_1_26
            gofumpt
            go-migrate
            (pinnedPkgs.golangci-lint)
            gopls
            gotestsum
            hadolint
            jq
            kubectl
            kubectx
            kubernetes-helm
            lazydocker
            lazygit
            less
            mise
            unstablePkgs.mockgen
            moreutils
            nfpm
            nix-prefetch-git
            nodejs
            openssh
            openssl
            pango
            pixman
            pkg-config
            pnpm
            postgresql_16
            proto_gen_go_1_30
            protobuf_23
            ripgrep
            shellcheck
            (pinnedPkgs.shfmt)
            # sqlc
            sqlc-custom
            syft
            terraform_1_15_5
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
          unstablePkgs.buildGo126Module {
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
      rec {
        inherit formatter;

        devShells = {
          default =
            (pkgs.mkShell.override (
              pkgs.lib.optionalAttrs pkgs.stdenv.isDarwin {
                stdenv = pkgs.overrideSDK pkgs.stdenv "12.3";
              }
            ))
            {
            buildInputs = devShellPackages;

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
