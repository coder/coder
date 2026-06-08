# kayla-provisionerd-key-file

Demo of the new `--key-file` flag (env `CODER_PROVISIONER_DAEMON_KEY_FILE`)
for `coder provisioner start`.

Recorded against `kayla/provisionerd-key-file` branch (commit `3d45136cdc`).

## What it shows

- A provisioner key file is created at `/tmp/key.txt` with `0600` perms.
- `coder provisioner start --help` now lists the new `--key-file` flag with
  its env-var equivalent and description.
- `coder provisioner start --key-file /tmp/key.txt --url http://example.com`
  accepts the flag (and fails downstream at auth, as expected against a
  non-Coder URL).

Addresses Kayla's complaint:

> "we don't support like CODER_PROVISIONER_DAEMON_KEY_FILE env var or
> anything as far as I can tell so my provisioner keys are just kinda
> sitting in plain text rn which I don't love"

![Demo](recording.gif)
