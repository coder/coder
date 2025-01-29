# (ThomasK33): Inlined the relevant dockerTools functions, so that we can
# set the maxLayers attribute on the attribute set passed
# to the buildNixShellImage function.
#
# I'll create an upstream PR to nixpkgs with those changes, making this
# eventually unnecessary and ripe for removal.
{
  lib,
  dockerTools,
  devShellTools,
  bashInteractive,
  fakeNss,
  runCommand,
  writeShellScriptBin,
  writeText,
  cacert,
  storeDir ? builtins.storeDir,
  pigz,
  zstd,
  stdenv,
  glibc,
}:
let
  inherit (lib)
    optionalString
    ;

  inherit (devShellTools)
    valueToString
    ;

  inherit (dockerTools)
    streamLayeredImage
    binSh
    usrBinEnv
    ;

  compressors = {
    none = {
      ext = "";
      nativeInputs = [ ];
      compress = "cat";
      decompress = "cat";
    };
    gz = {
      ext = ".gz";
      nativeInputs = [ pigz ];
      compress = "pigz -p$NIX_BUILD_CORES -nTR";
      decompress = "pigz -d -p$NIX_BUILD_CORES";
    };
    zstd = {
      ext = ".zst";
      nativeInputs = [ zstd ];
      compress = "zstd -T$NIX_BUILD_CORES";
      decompress = "zstd -d -T$NIX_BUILD_CORES";
    };
  };
  compressorForImage =
    compressor: imageName:
    compressors.${compressor}
      or (throw "in docker image ${imageName}: compressor must be one of: [${toString builtins.attrNames compressors}]");

  streamNixShellImage =
    {
      drv,
      name ? drv.name + "-env",
      tag ? null,
      uid ? 1000,
      gid ? 1000,
      homeDirectory ? "/build",
      shell ? bashInteractive + "/bin/bash",
      command ? null,
      run ? null,
      maxLayers ? 100,
      uname ? "nixbld",
    }:
    assert lib.assertMsg (!(drv.drvAttrs.__structuredAttrs or false))
      "streamNixShellImage: Does not work with the derivation ${drv.name} because it uses __structuredAttrs";
    assert lib.assertMsg (
      command == null || run == null
    ) "streamNixShellImage: Can't specify both command and run";
    let

      # A binary that calls the command to build the derivation
      builder = writeShellScriptBin "buildDerivation" ''
        exec ${lib.escapeShellArg (valueToString drv.drvAttrs.builder)} ${lib.escapeShellArgs (map valueToString drv.drvAttrs.args)}
      '';

      staticPath = "${dirOf shell}:${
        lib.makeBinPath (
          lib.flatten [
            builder
            drv.buildInputs
          ]
        )
      }";

      # https://github.com/NixOS/nix/blob/2.8.0/src/nix-build/nix-build.cc#L493-L526
      rcfile = writeText "nix-shell-rc" ''
        unset PATH
        dontAddDisableDepTrack=1
        # TODO: https://github.com/NixOS/nix/blob/2.8.0/src/nix-build/nix-build.cc#L506
        [ -e $stdenv/setup ] && source $stdenv/setup
        PATH=${staticPath}:"$PATH"
        SHELL=${lib.escapeShellArg shell}
        BASH=${lib.escapeShellArg shell}
        set +e
        [ -n "$PS1" -a -z "$NIX_SHELL_PRESERVE_PROMPT" ] && PS1='\n\[\033[1;32m\][nix-shell:\w]\$\[\033[0m\] '
        if [ "$(type -t runHook)" = function ]; then
          runHook shellHook
        fi
        unset NIX_ENFORCE_PURITY
        shopt -u nullglob
        shopt -s execfail
        ${optionalString (command != null || run != null) ''
          ${optionalString (command != null) command}
          ${optionalString (run != null) run}
          exit
        ''}
      '';

      nixConfFile = writeText "nix-conf" ''
        experimental-features = nix-command flakes
      '';

      etcNixConf = runCommand "etcd-nix-conf" { } ''
        mkdir -p $out/etc/nix/
        ln -s ${nixConfFile} $out/etc/nix/nix.conf
      '';

      # https://github.com/NixOS/nix/blob/2.8.0/src/libstore/globals.hh#L464-L465
      sandboxBuildDir = "/build";

      drvEnv =
        devShellTools.unstructuredDerivationInputEnv { inherit (drv) drvAttrs; }
        // devShellTools.derivationOutputEnv {
          outputList = drv.outputs;
          outputMap = drv;
        };

      # Environment variables set in the image
      envVars =
        {

          # Root certificates for internet access
          SSL_CERT_FILE = "${cacert}/etc/ssl/certs/ca-bundle.crt";
          NIX_SSL_CERT_FILE = "${cacert}/etc/ssl/certs/ca-bundle.crt";

          # https://github.com/NixOS/nix/blob/2.8.0/src/libstore/build/local-derivation-goal.cc#L1027-L1030
          # PATH = "/path-not-set";
          # Allows calling bash and `buildDerivation` as the Cmd
          PATH = staticPath;

          # https://github.com/NixOS/nix/blob/2.8.0/src/libstore/build/local-derivation-goal.cc#L1032-L1038
          HOME = homeDirectory;

          # https://github.com/NixOS/nix/blob/2.8.0/src/libstore/build/local-derivation-goal.cc#L1040-L1044
          NIX_STORE = storeDir;

          # https://github.com/NixOS/nix/blob/2.8.0/src/libstore/build/local-derivation-goal.cc#L1046-L1047
          # TODO: Make configurable?
          NIX_BUILD_CORES = "1";

          # Make sure we get the libraries for C and C++ in.
          LD_LIBRARY_PATH = lib.makeLibraryPath [ stdenv.cc.cc ];
        }
        // drvEnv
        // {

          # https://github.com/NixOS/nix/blob/2.8.0/src/libstore/build/local-derivation-goal.cc#L1008-L1010
          NIX_BUILD_TOP = sandboxBuildDir;

          # https://github.com/NixOS/nix/blob/2.8.0/src/libstore/build/local-derivation-goal.cc#L1012-L1013
          TMPDIR = sandboxBuildDir;
          TEMPDIR = sandboxBuildDir;
          TMP = sandboxBuildDir;
          TEMP = "/tmp";

          # https://github.com/NixOS/nix/blob/2.8.0/src/libstore/build/local-derivation-goal.cc#L1015-L1019
          PWD = homeDirectory;

          # https://github.com/NixOS/nix/blob/2.8.0/src/libstore/build/local-derivation-goal.cc#L1071-L1074
          # We don't set it here because the output here isn't handled in any special way
          # NIX_LOG_FD = "2";

          # https://github.com/NixOS/nix/blob/2.8.0/src/libstore/build/local-derivation-goal.cc#L1076-L1077
          TERM = "xterm-256color";
        };

    in
    streamLayeredImage {
      inherit name tag maxLayers;
      contents = [
        binSh
        usrBinEnv
        etcNixConf
        (fakeNss.override {
          # Allows programs to look up the build user's home directory
          # https://github.com/NixOS/nix/blob/ffe155abd36366a870482625543f9bf924a58281/src/libstore/build/local-derivation-goal.cc#L906-L910
          # Slightly differs however: We use the passed-in homeDirectory instead of sandboxBuildDir.
          # We're doing this because it's arguably a bug in Nix that sandboxBuildDir is used here: https://github.com/NixOS/nix/issues/6379
          extraPasswdLines = [
            "${toString uname}:x:${toString uid}:${toString gid}:Build user:${homeDirectory}:${lib.escapeShellArg shell}"
          ];
          extraGroupLines = [
            "${toString uname}:!:${toString gid}:"
          ];
        })
      ];

      fakeRootCommands = ''
        # Effectively a single-user installation of Nix, giving the user full
        # control over the Nix store. Needed for building the derivation this
        # shell is for, but also in case one wants to use Nix inside the
        # image
        mkdir -p ./nix/{store,var/nix} ./etc/nix
        chown -R ${toString uid}:${toString gid} ./nix ./etc/nix

        # Gives the user control over the build directory
        mkdir -p .${sandboxBuildDir}
        chown -R ${toString uid}:${toString gid} .${sandboxBuildDir}

        mkdir -p .${homeDirectory}
        chown -R ${toString uid}:${toString gid} .${homeDirectory}

        mkdir -p ./tmp
        chown -R ${toString uid}:${toString gid} ./tmp

        mkdir -p ./etc/skel
        chown -R ${toString uid}:${toString gid} ./etc/skel

        # Create traditional /lib or /lib64 as needed.
        # For aarch64 (arm64):
        if [ -e "${glibc}/lib/ld-linux-aarch64.so.1" ]; then
          mkdir -p ./lib
          ln -s "${glibc}/lib/ld-linux-aarch64.so.1" ./lib/ld-linux-aarch64.so.1
        fi

        # For x86_64:
        if [ -e "${glibc}/lib64/ld-linux-x86-64.so.2" ]; then
          mkdir -p ./lib64
          ln -s "${glibc}/lib64/ld-linux-x86-64.so.2" ./lib64/ld-linux-x86-64.so.2
        fi
      '';

      # Run this image as the given uid/gid
      config.User = "${toString uid}:${toString gid}";
      config.Cmd =
        # https://github.com/NixOS/nix/blob/2.8.0/src/nix-build/nix-build.cc#L185-L186
        # https://github.com/NixOS/nix/blob/2.8.0/src/nix-build/nix-build.cc#L534-L536
        if run == null then
          [
            shell
            "--rcfile"
            rcfile
          ]
        else
          [
            shell
            rcfile
          ];
      config.WorkingDir = homeDirectory;
      config.Env = lib.mapAttrsToList (name: value: "${name}=${value}") envVars;
    };
in
{
  inherit streamNixShellImage;

  # This function streams a docker image that behaves like a nix-shell for a derivation
  # Docs: doc/build-helpers/images/dockertools.section.md
  # Tests: nixos/tests/docker-tools-nix-shell.nix

  # Wrapper around streamNixShellImage to build an image from the result
  # Docs: doc/build-helpers/images/dockertools.section.md
  # Tests: nixos/tests/docker-tools-nix-shell.nix
  buildNixShellImage =
    {
      drv,
      compressor ? "gz",
      ...
    }@args:
    let
      stream = streamNixShellImage (builtins.removeAttrs args [ "compressor" ]);
      compress = compressorForImage compressor drv.name;
    in
    runCommand "${drv.name}-env.tar${compress.ext}" {
      inherit (stream) imageName;
      passthru = { inherit (stream) imageTag; };
      nativeBuildInputs = compress.nativeInputs;
    } "${stream} | ${compress.compress} > $out";
}
