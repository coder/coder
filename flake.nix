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
          gnused
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
          strace
          terraform
          typos
          vim
          yq
          zip
          zstd
        ];

        # Start with an Ubuntu image!
        baseDevEnvImage = pkgs.dockerTools.pullImage {
          imageName = "ubuntu";
          imageDigest = "sha256:7a520eeb6c18bc6d32a21bb7edcf673a7830813c169645d51c949cecb62387d0";
          sha256 = "ajZzFSG/q7F5wAXfBOPpYBT+aVy8lqAXtBzkmAe2SeE=";
          finalImageName = "ubuntu";
          finalImageTag = "lunar";
        };
        # Build the image and modify it to have the "coder" user.
        intermediateDevEnvImage = pkgs.dockerTools.buildImage {
          name = "intermediate";
          fromImage = baseDevEnvImage;
          # This replaces the "ubuntu" user with "coder" and
          # gives it sudo privileges!
          runAsRoot = ''
            #!${pkgs.runtimeShell}
            ${pkgs.dockerTools.shadowSetup}
            userdel ubuntu
            useradd coder \
              --create-home \
              --shell=/bin/bash \
              --uid=1000 \
              --user-group
            cat > /etc/pam.d/other <<EOF
                account sufficient pam_unix.so
                auth sufficient pam_rootok.so
                password requisite pam_unix.so nullok yescrypt
                session required pam_unix.so
            EOF
            echo "coder ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers
            mkdir -p /etc/ssl/certs
            cp -r ${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt /etc/ssl/certs/ca-certificates.crt
            cp ${pkgs.sudo}/bin/sudo /usr/bin/sudo
            chmod 4755 /usr/bin/sudo
          '';
        };
        devEnvPath = "PATH=${pkgs.lib.makeBinPath devShellPackages}:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/home/coder/go/bin";
        devEnvImage = pkgs.dockerTools.streamLayeredImage {
          name = "codercom/oss-dogfood";
          tag = "testing";
          fromImage = intermediateDevEnvImage;
          contents = [
            (
              pkgs.writeTextDir
                "etc/environment"
                ''
                  ${devEnvPath}
                ''
            )
          ];

          config = {
            Env = [
              devEnvPath
              #This setting prevents Go from using the public checksum database for
              # our module path prefixes. It is required because these are in private
              # repositories that require authentication.
              #
              # For details, see: https://golang.org/ref/mod#private-modules
              "GOPRIVATE=coder.com,cdr.dev,go.coder.com,github.com/cdr,github.com/coder"
              # Increase memory allocation to NodeJS
              "NODE_OPTIONS=--max_old_space_size=8192"
            ];
            Entrypoint = [ "/bin/bash" ];
            User = "coder";
          };
        };
      in
      {
        packages = {
          devEnvironmentDocker = devEnvImage;
          # other packages you want to define for this system
        };
        defaultPackage = formatter; # or replace it with your desired default package.
        devShell = pkgs.mkShell { buildInputs = devShellPackages; };
      }
    );
}
