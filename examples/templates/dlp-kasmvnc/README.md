# dlp-kasmvnc

A KasmVNC remote desktop wrapped in the strict DLP policy. Only the
`kasm-vnc` coder_app slug is allow-listed; SSH, the dashboard web
terminal, port forwarding, and every other coder_app are blocked.

## Build and push

The dev server must already be running with `dev_overrides` configured
for the local `terraform-provider-coder` build (see the prototype
branch's top-level setup notes).

```bash
cd "$(git rev-parse --show-toplevel)"
./scripts/coder-dev.sh templates push dlp-kasmvnc \
  --directory examples/templates/dlp-kasmvnc --yes
./scripts/coder-dev.sh create kasm1 --template dlp-kasmvnc --yes
```

## First boot is slow

The base image is `codercom/enterprise-base:ubuntu`, which does not
include a desktop environment. The agent installs `xfce4`,
`xfce4-terminal`, and `dbus-x11` on first boot and writes a marker file
so subsequent restarts skip the install. Expect a few minutes the first
time before the KasmVNC app shows up.

## Verifying the policy

Try `coder ssh kasm1`: should fail with the DLP CLI denial, since
`ssh_access=false`. The dashboard web terminal, Ports tab, and Desktop
button (backed by the portabledesktop module) should return 403. The
"KasmVNC" app should load normally because the `kasm-vnc` slug is in
`allowed_applications`.
