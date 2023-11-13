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
        # Check in https://search.nixos.org/packages to find new packages.
        # Use `nix --extra-experimental-features nix-command --extra-experimental-features flakes flake update`
        # to update the lock file if packages are out-of-date.

        # From https://nixos.wiki/wiki/Google_Cloud_SDK
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
          go_1_20
          go-migrate
          golangci-lint
          gopls
          gotestsum
          jq
          kubectl
          kubectx
          kubernetes-helm
          less
          # Needed for many LD system libs!
          libuuid
          mockgen
          nfpm
          nodejs
          nodePackages.pnpm
          nodePackages.prettier
          nodePackages.typescript
          nodePackages.typescript-language-server
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
          # strace is not available on OSX
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
        # We separate these to reduce the size of the dev shell for packages that we only
        # want in the image.
        devImagePackages = with pkgs; [
          docker
          exa
          freetype
          glib
          harfbuzz
          nix
          nixpkgs-fmt
          screen
        ];

        # This is the base image for our Docker container used for development.
        # Use `nix-prefetch-docker ubuntu --arch amd64 --image-tag lunar` to get this.
        baseDevEnvImage = pkgs.dockerTools.pullImage {
          imageName = "ubuntu";
          imageDigest = "sha256:7a520eeb6c18bc6d32a21bb7edcf673a7830813c169645d51c949cecb62387d0";
          sha256 = "ajZzFSG/q7F5wAXfBOPpYBT+aVy8lqAXtBzkmAe2SeE=";
          finalImageName = "ubuntu";
          finalImageTag = "lunar";
        };
        # This is an intermediate stage that adds sudo with the setuid bit set.
        # Nix doesn't allow setuid binaries in the store, so we have to do this
        # in a separate stage.
        intermediateDevEnvImage = pkgs.dockerTools.buildImage {
          name = "intermediate";
          fromImage = baseDevEnvImage;
          runAsRoot = ''
            #!${pkgs.runtimeShell}
            ${pkgs.dockerTools.shadowSetup}
            userdel ubuntu
            groupadd docker
            useradd coder \
              --create-home \
              --shell=/bin/bash \
              --uid=1000 \
              --user-group \
              --groups docker
            cp ${pkgs.sudo}/bin/sudo usr/bin/sudo
            chmod 4755 usr/bin/sudo
            mkdir -p /etc/init.d
          '';
        };
        allPackages = devShellPackages ++ devImagePackages;
        # Environment variables that live in `/etc/environment` in the container.
        # These will also be applied to the container config.
        devEnvVars = [
          "PATH=${pkgs.lib.makeBinPath (allPackages)}:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/home/coder/go/bin"
          "LD_LIBRARY_PATH=${pkgs.lib.makeLibraryPath allPackages}"
          # This setting prevents Go from using the public checksum database for
          # our module path prefixes. It is required because these are in private
          # repositories that require authentication.
          #
          # For details, see: https://golang.org/ref/mod#private-modules
          "GOPRIVATE=coder.com,cdr.dev,go.coder.com,github.com/cdr,github.com/coder"
          # Increase memory allocation to NodeJS
          "NODE_OPTIONS=--max_old_space_size=8192"
          "TERM=xterm-256color"
          "LANG=en_US.UTF-8"
          "LOCALE_ARCHIVE=/usr/lib/locale/locale-archive"
        ];
        # Builds our development environment image with all the tools included.
        # Using Nix instead of Docker is **significantly** faster. This _build_
        # doesn't really build anything, it just copies pre-built binaries into
        # a container and adds them to the $PATH.
        #
        # To test changes and iterate on this, you can run:
        # > nix build .#devEnvImage && ./result | docker load
        # This will import the image into your local Docker daemon.
        devEnvImage = pkgs.dockerTools.streamLayeredImage {
          name = "codercom/oss-dogfood";
          tag = "latest";
          fromImage = intermediateDevEnvImage;
          maxLayers = 64;
          contents = [
            # Required for `sudo` to persist the proper `PATH`.
            (
              pkgs.writeTextDir "etc/environment" (pkgs.lib.strings.concatLines devEnvVars)
            )
            # Allows `coder` to use `sudo` without a password.
            (
              pkgs.writeTextDir "etc/sudoers" ''
                coder ALL=(ALL) NOPASSWD:ALL
              ''
            )
            # Also allows `coder` to use `sudo` without a password.
            (
              pkgs.writeTextDir "etc/pam.d/other" ''
                account sufficient pam_unix.so
                auth sufficient pam_rootok.so
                password requisite pam_unix.so nullok yescrypt
                session required pam_unix.so
              ''
            )
            # This allows users to chsh.
            (
              pkgs.writeTextDir "etc/pam.d/chsh" ''
                auth        sufficient  pam_rootok.so
              ''
            )
            # The default Nix config!
            (
              pkgs.writeTextDir "etc/nix/nix.conf" ''
                experimental-features = nix-command flakes
              ''
            )
            # Allow people to change shells!
            (
              pkgs.writeTextDir "etc/shells" ''
                /bin/bash
                ${pkgs.zsh}/bin/zsh
              ''
            )
            # This is the debian script for managing Docker with `sudo service docker ...`.
            (
              pkgs.writeTextFile {
                name = "docker";
                destination = "/etc/init.d/docker";
                executable = true;
                text = (builtins.readFile (
                  pkgs.fetchFromGitHub
                    {
                      owner = "moby";
                      repo = "moby";
                      rev = "ae737656f9817fbd5afab96aa083754cfb81aab0";
                      sha256 = "sha256-oS3WplsxhKHCuHwL4/ytsCNJ1N/SZhlUZmzZTf81AoE=";
                    } + "/contrib/init/sysvinit-debian/docker"
                ));
              }
            )
            # The Docker script above looks here for the daemon binary location.
            # Because we're injecting it with Nix, it's not in the default spot.
            (
              pkgs.writeTextDir "etc/default/docker" ''
                DOCKERD=${pkgs.docker}/bin/dockerd
              ''
            )
            # The same as `sudo apt install ca-certificates -y'.
            (
              pkgs.writeTextDir "etc/ssl/certs/ca-certificates.crt"
                (builtins.readFile "${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt")
            )
          ];
          # Required for the UTF-8 locale to exist!
          extraCommands = ''
            mkdir -p usr/lib/locale
            cp -a ${pkgs.glibcLocales}/lib/locale/locale-archive usr/lib/locale/locale-archive
          '';

          config = {
            Env = devEnvVars;
            Entrypoint = [ "/bin/bash" ];
            User = "coder";
          };
        };
      in
      {
        packages = {
          devEnvImage = devEnvImage;
        };
        defaultPackage = formatter; # or replace it with your desired default package.
        devShell = pkgs.mkShell { buildInputs = devShellPackages; };
      }
    );
}
