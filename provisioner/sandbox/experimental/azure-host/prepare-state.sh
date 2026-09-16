#!/usr/bin/env bash
# This service must finish before the outer agent can launch any bootstrap work.
set -euo pipefail
umask 077

state=/var/lib/coder-sandbox-host
device=/dev/disk/azure/scsi1/lun10
expected_label=coder-sbx-state

fail() {
  printf 'State disk setup failed: %s\n' "$*" >&2
  exit 1
}

for ((attempt = 0; attempt < 300; attempt++)); do
  if [[ -b $device ]]; then break; fi
  sleep 2
done
[[ -b $device ]] || fail 'Azure data disk LUN10 did not appear within 600 seconds.'

filesystem=$(blkid -p -s TYPE -o value "$device" || true)
if [[ -z $filesystem ]]; then
  # Never replace an existing partition table or unknown filesystem signature.
  signatures=$(wipefs --no-act --noheadings --output TYPE "$device")
  [[ -z $signatures ]] || fail 'LUN10 has an unrecognized disk signature; leaving it intact.'
  mkfs.ext4 -L "$expected_label" "$device"
elif [[ $filesystem != ext4 ]]; then
  fail "Expected ext4 on LUN10, found $filesystem; leaving it intact."
fi

label=$(blkid -s LABEL -o value "$device")
[[ $label == "$expected_label" ]] || fail 'LUN10 does not have the experiment state label.'
uuid=$(blkid -s UUID -o value "$device")
[[ -n $uuid ]] || fail 'The state filesystem has no UUID.'
install -d -m 0755 "$state"

if mountpoint --quiet "$state"; then
  mounted_uuid=$(findmnt --noheadings --output UUID --target "$state")
  [[ $mounted_uuid == "$uuid" ]] || fail 'A different filesystem is already mounted at the state path.'
else
  mount -t ext4 "$device" "$state"
fi

fstab_entry="UUID=$uuid $state ext4 defaults,x-systemd.device-timeout=600 0 2"
if ! grep --fixed-strings --line-regexp --quiet "$fstab_entry" /etc/fstab; then
  if awk -v target="$state" '$1 !~ /^#/ && $2 == target {found=1} END {exit !found}' /etc/fstab; then
    fail 'A conflicting fstab entry exists for the state mount.'
  fi
  printf '%s\n' "$fstab_entry" >> /etc/fstab
fi

# OS disks are replaced on workspace start. Keep tools, configuration and
# delivered assets beside the database and native runtime state.
for entry in 'tools:/opt/coder-sandbox-host' 'config:/etc/coder-sandbox-host' 'bootstrap:/opt/coder-sandbox-bootstrap'; do
  directory=${entry%%:*}
  link=${entry#*:}
  target=$state/$directory
  install -d -m 0755 "$target"
  if [[ -L $link ]]; then
    [[ $(readlink "$link") == "$target" ]] || fail "Unexpected symlink at $link."
  elif [[ -e $link ]]; then
    fail "Existing non-symlink path at $link; leaving it intact."
  else
    ln -s "$target" "$link"
  fi
done

printf 'Persistent sandbox state mounted successfully at %s.\n' "$state"
