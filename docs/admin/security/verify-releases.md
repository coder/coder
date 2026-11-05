---
title: Verify Coder releases
---

This guide is for Coder deployment administrators who download Coder release artifacts.
Use it to confirm that an artifact came from the Coder release pipeline and that nobody changed it after the build.
Verify an artifact before you install it or copy it into an internal mirror or registry.

Each Coder release publishes the following verification data:

- A checksum file signed with the Coder release key, a GNU Privacy Guard (GPG) key.
  It covers every other file that the release pipeline uploads to the GitHub release.
- Build provenance attestations in the [Supply-chain Levels for Software Artifacts (SLSA) provenance](https://slsa.dev/spec/v1.0/provenance) format, published as [GitHub artifact attestations](https://docs.github.com/en/actions/concepts/security/artifact-attestations).
- A software bill of materials (SBOM) for the container image in the [Software Package Data Exchange (SPDX)](https://spdx.dev/) JSON format.
  It's attached to the image as a signed attestation and to the GitHub release as a file.
- Detached GPG signatures for the standalone CLI binaries on `releases.coder.com`.
- Operating system code signatures on the Windows and macOS binaries.

The release pipeline creates provenance attestations on a best-effort basis, and a release can publish without them.
The signed checksum file is the verification method that covers every file the release pipeline uploads to the GitHub release.
The **Source code** archives that GitHub generates for each release aren't in the checksum file.

## What you can verify

| Artifact                                                  | Where to get it                                                             | How to verify it                |
|-----------------------------------------------------------|-----------------------------------------------------------------------------|---------------------------------|
| Archives, packages, and the Windows installer             | GitHub release                                                              | Signed checksums, provenance    |
| Helm charts for Coder, provisioners, and AI Gateway       | GitHub release, `https://helm.coder.com/v2`, or `oci://ghcr.io/coder/chart` | Signed checksums, provenance    |
| SBOM file and offline docs                                | GitHub release                                                              | Signed checksums                |
| Container image `ghcr.io/coder/coder:v<version>`          | GitHub Container Registry                                                   | Provenance, SBOM attestation    |
| Standalone CLI binaries, such as `coder-linux-amd64`      | `https://releases.coder.com/coder-cli/<version>/`                           | Detached GPG signature          |
| CLI binaries in the Windows and macOS archives, installer | GitHub release                                                              | Operating system code signature |

## Prerequisites

Install the tools for the checks you plan to run:

- [GnuPG](https://gnupg.org/download/) for the checksum file and CLI binary signatures.
  On Windows, install [Gpg4win](https://www.gpg4win.org/), which adds `gpg` to PowerShell.
- The [GitHub CLI](https://cli.github.com/), signed in with `gh auth login`, for provenance attestations and release downloads.
- [Cosign](https://docs.sigstore.dev/cosign/system_config/installation/) version 2 or 3 for the container image SBOM.
- On Linux and macOS, `jq` to extract the SBOM from the attestation.

The commands on this page use a `VERSION` variable.
Set it to the release you want to verify, without the leading `v` (for example, `2.38.6`):

<div class="tabs">

### Linux and macOS

```sh
VERSION="<version>"
```

### Windows

```ps1
$VERSION = "<version>"
```

</div>

Commands on this page that show no Windows variant run unchanged in PowerShell.

## Import the Coder release key

The Coder release key signs the checksum file and the standalone CLI binaries.

1. Download the Coder release key:

   <div class="tabs">

   ### Linux and macOS

   ```sh
   curl -fsSLO https://raw.githubusercontent.com/coder/coder/main/release.key
   ```

   ### Windows

   ```ps1
   Invoke-WebRequest -Uri https://raw.githubusercontent.com/coder/coder/main/release.key -OutFile release.key
   ```

   </div>

2. Display the key and its fingerprint:

   ```sh
   gpg --show-keys --with-fingerprint release.key
   ```

   The user ID is `Coder Release Signing Key <security@coder.com>`.
   GPG also prints `WARNING: No valid encryption subkey left over.`
   The warning is about the key's encryption subkey, which signature checks don't use.

3. Confirm that the primary key fingerprint matches the published fingerprint:

   ```txt
   TODO(security): publish the confirmed primary key fingerprint here.
   ```

4. Import the key:

   ```sh
   gpg --import release.key
   ```

   The output confirms the import:

   ```txt
   gpg: key <key ID>: public key "Coder Release Signing Key <security@coder.com>" imported
   ```

## Verify a release file with GPG

The `coder_<version>_checksums.txt` file lists the SHA-256 hash of every other file that the release pipeline uploads to the GitHub release.
The `coder_<version>_checksums.txt.asc` file is its detached GPG signature.

1. Import the Coder release key, as [Import the Coder release key](#import-the-coder-release-key) describes.
2. Download the checksum file, its signature, and the artifact you want to verify.

   If your artifact isn't `linux_amd64.tar.gz`, replace that suffix, for example with `darwin_arm64.zip` or `windows_amd64.zip`:

   ```sh
   gh release download "v${VERSION}" --repo coder/coder --pattern "coder_${VERSION}_checksums.txt*" --pattern "coder_${VERSION}_linux_amd64.tar.gz"
   ```

3. Verify the signature on the checksum file:

   ```sh
   gpg --verify "coder_${VERSION}_checksums.txt.asc" "coder_${VERSION}_checksums.txt"
   ```

   The output includes a good signature from the Coder release key:

   ```txt
   gpg: Good signature from "Coder Release Signing Key <security@coder.com>" [unknown]
   ```

   GPG also warns that the key is not certified with a trusted signature.
   The warning means that you haven't marked the key as trusted in your local keyring.
   The fingerprint check when you imported the key is what ties the key to Coder.

4. Check the artifact's hash against the checksum file.

   If you downloaded a different artifact, replace the file name in the command:

   <div class="tabs">

   ### Linux

   ```sh
   grep "  coder_${VERSION}_linux_amd64.tar.gz$" "coder_${VERSION}_checksums.txt" | sha256sum --check
   ```

   ### macOS

   ```sh
   grep "  coder_${VERSION}_darwin_arm64.zip$" "coder_${VERSION}_checksums.txt" | shasum -a 256 --check
   ```

   ### Windows

   ```ps1
   $File = "coder_${VERSION}_windows_amd64.zip"
   $Expected = (Select-String -Path "coder_${VERSION}_checksums.txt" -SimpleMatch "  $File").Line.Split(" ")[0]
   $Actual = (Get-FileHash -Algorithm SHA256 $File).Hash.ToLower()
   if ($Actual -eq $Expected) { "${File}: OK" } else { "${File}: FAILED" }
   ```

   </div>

   The command prints the file name followed by `OK`:

   ```txt
   coder_<version>_linux_amd64.tar.gz: OK
   ```

If the signature check or the hash check fails, do not install the artifact.

## Verify a standalone CLI binary with GPG

Coder publishes standalone CLI binaries for each release at `https://releases.coder.com/coder-cli/<version>/`, each with a detached `.asc` signature.
The binaries are named `coder-<os>-<arch>`:

- `coder-darwin-amd64` and `coder-darwin-arm64`
- `coder-linux-amd64`, `coder-linux-arm64`, and `coder-linux-armv7`
- `coder-windows-amd64.exe` and `coder-windows-arm64.exe`

1. Import the Coder release key, as [Import the Coder release key](#import-the-coder-release-key) describes.
2. Set `BINARY` to the binary for your platform, then download the binary and its signature:

   <div class="tabs">

   ### Linux and macOS

   ```sh
   BINARY="coder-linux-amd64"
   curl -fsSLO "https://releases.coder.com/coder-cli/${VERSION}/${BINARY}"
   curl -fsSLO "https://releases.coder.com/coder-cli/${VERSION}/${BINARY}.asc"
   ```

   ### Windows

   ```ps1
   $BINARY = "coder-windows-amd64.exe"
   Invoke-WebRequest -Uri "https://releases.coder.com/coder-cli/${VERSION}/${BINARY}" -OutFile $BINARY
   Invoke-WebRequest -Uri "https://releases.coder.com/coder-cli/${VERSION}/${BINARY}.asc" -OutFile "${BINARY}.asc"
   ```

   </div>

3. Verify the signature:

   ```sh
   gpg --verify "${BINARY}.asc" "${BINARY}"
   ```

   The output includes `Good signature from "Coder Release Signing Key <security@coder.com>"`.

## Verify build provenance with the GitHub CLI

A provenance attestation records which repository, workflow, and Git tag built an artifact.
The GitHub CLI checks that the attestation is signed and that it matches the artifact's digest.

Provenance attestations can exist for the container image, the archives, the packages, the Windows installer, and the Helm charts.
The checksum file, the SBOM file, the offline docs, and the standalone CLI binaries have no provenance attestation.

### Verify a release file

Run `gh attestation verify` on a file that you downloaded from the GitHub release or from `https://helm.coder.com/v2`:

```sh
gh attestation verify "coder_${VERSION}_linux_amd64.tar.gz" --repo coder/coder --signer-workflow coder/coder/.github/workflows/release.yaml --source-ref "refs/tags/v${VERSION}"
```

When the attestation matches, the output ends with the build workflow and the release tag:

```txt
✓ Verification succeeded!

The following 1 attestation matched the policy criteria

- Attestation #1
  - Build repo:..... coder/coder
  - Build workflow:. .github/workflows/release.yaml@refs/tags/v<version>
  - Signer repo:.... coder/coder
  - Signer workflow: .github/workflows/release.yaml@refs/tags/v<version>
```

A Helm chart that you pull from `oci://ghcr.io/coder/chart` with `helm pull` has a different file name, such as `coder-<version>.tgz`, and the same contents.
Run the same command on that file.

If GitHub has no attestation for the file's digest, the command fails with `HTTP 404: Not Found`.
For a file on the GitHub release, verify it with the [signed checksum file](#verify-a-release-file-with-gpg) instead.

### Verify the container image

Pass the image reference with the `oci://` prefix:

```sh
gh attestation verify "oci://ghcr.io/coder/coder:v${VERSION}" --repo coder/coder --signer-workflow coder/coder/.github/workflows/release.yaml --source-ref "refs/tags/v${VERSION}"
```

The output has the same format as the output for a release file.
The tag points to the multi-architecture image index, and the attestation covers that index digest.

If the command fails with `HTTP 404: Not Found`, verify the [SBOM attestation](#verify-the-container-image-sbom-with-cosign) instead.
Its signing certificate also identifies the Coder release workflow and the release tag.

## Verify the container image SBOM with Cosign

The release pipeline generates an SPDX SBOM for `ghcr.io/coder/coder:v<version>` and signs it as an attestation on the image with Sigstore keyless signing.
The signing certificate identifies the Coder release workflow and the release tag.

1. Verify the SBOM attestation and save the SBOM.

   If you use Cosign 2, remove the `--new-bundle-format=false` line from the command.
   Cosign 3 needs the flag to read the older attestation format that the SBOM attestation uses, and it prints a warning that the flag is deprecated.

   <div class="tabs">

   ### Linux and macOS

   ```sh
   cosign verify-attestation \
     --new-bundle-format=false \
     --type spdxjson \
     --certificate-identity "https://github.com/coder/coder/.github/workflows/release.yaml@refs/tags/v${VERSION}" \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com \
     "ghcr.io/coder/coder:v${VERSION}" \
     | jq -r '.payload' | base64 -d | jq '.predicate' > "coder_${VERSION}_image_sbom.spdx.json"
   ```

   ### Windows

   ```ps1
   $Envelope = cosign verify-attestation `
     --new-bundle-format=false `
     --type spdxjson `
     --certificate-identity "https://github.com/coder/coder/.github/workflows/release.yaml@refs/tags/v${VERSION}" `
     --certificate-oidc-issuer https://token.actions.githubusercontent.com `
     "ghcr.io/coder/coder:v${VERSION}" | ConvertFrom-Json
   $Statement = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($Envelope.payload)) | ConvertFrom-Json
   $Statement.predicate | ConvertTo-Json -Depth 100 | Set-Content "coder_${VERSION}_image_sbom.spdx.json"
   ```

   </div>

   Cosign prints the checks it performed and the certificate details to the terminal.
   The output includes these lines:

   ```txt
   The following checks were performed on each of these signatures:
     - The cosign claims were validated
     - Existence of the claims in the transparency log was verified offline
     - The code-signing certificate was verified using trusted certificate authority certificates
   Certificate subject: https://github.com/coder/coder/.github/workflows/release.yaml@refs/tags/v<version>
   Certificate issuer URL: https://token.actions.githubusercontent.com
   ```

2. Open `coder_<version>_image_sbom.spdx.json` in your SBOM or vulnerability scanning tool.

The same SBOM is attached to the GitHub release as `coder_<version>_sbom.spdx.json`, and the signed checksum file covers it.
When the release is the highest Coder version that isn't a release candidate, the release pipeline also publishes `ghcr.io/coder/coder:latest`.
The release then includes `coder_latest_sbom.spdx.json` for that image.

## Check operating system code signatures

Coder signs the Windows CLI binary and the Windows installer with an Extended Validation (EV) code signing certificate.
Coder signs the macOS CLI binary with an Apple Developer certificate and notarizes the macOS archives with Apple.
These signatures let the operating system check the publisher when you run the binary.

<div class="tabs">

### macOS

Extract `coder` from the `coder_<version>_darwin_<arch>.zip` archive, then display its signature:

```sh
codesign -dvv coder
```

The signing authority is `TODO(security): confirm the Apple Developer ID and Team ID to publish`.

If the Mac has internet access, check that Gatekeeper accepts the notarized binary:

```sh
spctl --assess -vvv -t install coder
```

The archive doesn't include the notarization ticket, so `spctl` downloads the ticket from Apple.
Without internet access, the check fails even for a genuine binary.

### Windows

Display the signature on the installer or on `coder.exe` from the `coder_<version>_windows_<arch>.zip` archive:

```ps1
Get-AuthenticodeSignature ".\coder_${VERSION}_windows_amd64_installer.exe" | Format-List Status, SignerCertificate
```

The `Status` is `Valid`.
The signer certificate subject is `TODO(security): confirm the EV certificate subject to publish`.

</div>

## Learn more

- [Security advisories](./index.md#security-advisories)
- [`gh attestation verify` reference](https://cli.github.com/manual/gh_attestation_verify)
- [Sigstore documentation](https://docs.sigstore.dev/)
- [Coder release key](../../../release.key)
