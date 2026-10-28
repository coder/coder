#!/bin/sh
set -eu
umask 077

# This script comes from the new Secret disk on every workspace start.
# /run is volatile. Never fall back to credentials on the persistent root disk.
rm -f /run/coder-agent.env /run/coder-agent-init.sh
test -s /run/coder/credentials/agent.env
test -s /run/coder/credentials/init.sh
install -o root -g root -m 0600 /run/coder/credentials/agent.env /run/coder-agent.env
install -o coder -g coder -m 0700 /run/coder/credentials/init.sh /run/coder-agent-init.sh

install -d -m 0755 /run/systemd/system
cat >/run/systemd/system/coder-agent.service <<'UNIT'
[Unit]
Description=Coder workspace agent
Wants=network-online.target docker.service
Requires=coder-bootstrap.service
After=network-online.target coder-bootstrap.service docker.service

[Service]
User=coder
Group=coder
WorkingDirectory=/home/coder
EnvironmentFile=/run/coder-agent.env
ExecStart=/bin/sh /run/coder-agent-init.sh
Restart=always
RestartSec=5
TimeoutStopSec=90
UNIT
chmod 0644 /run/systemd/system/coder-agent.service
systemctl daemon-reload
# The agent waits for this oneshot unit to finish before it starts.
systemctl start --no-block coder-agent.service
