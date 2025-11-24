#!/bin/sh
set -e

# Docker-in-Docker setup for Coder dev containers using host.docker.internal
# URLs. When Docker runs inside a container, the "docker0" bridge interface
# can interfere with host.docker.internal DNS resolution, breaking
# connectivity to the Coder server.

if [ "${CODER_AGENT_URL#*host.docker.internal}" = "$CODER_AGENT_URL" ]; then
	# External access URL detected, no networking workarounds needed.
	sudo service docker start
	exit 0
fi

# host.docker.internal URL detected. Docker's default bridge network creates
# a "docker0" interface that can shadow the host.docker.internal hostname
# resolution. This typically happens when Docker starts inside a devcontainer,
# as the inner Docker daemon creates its own bridge network that conflicts
# with the outer one.

# Enable IP forwarding to allow packets to route between the host network and
# the devcontainer networks. Without this, traffic cannot flow properly
# between the different Docker bridge networks.
echo 1 | sudo tee /proc/sys/net/ipv4/ip_forward
sudo iptables -t nat -A POSTROUTING -j MASQUERADE

# Set up port forwarding to the host Docker gateway (typically 172.17.0.1).
# We resolve host.docker.internal to get the actual IP and create NAT rules
# to forward traffic from this workspace to the host.
host_ip=$(getent hosts host.docker.internal | awk '{print $1}')

echo "Host IP for host.docker.internal: $host_ip"

# Extract the port from CODER_AGENT_URL. The URL format is typically
# http://host.docker.internal:port/.
port="${CODER_AGENT_URL##*:}"
port="${port%%/*}"
case "$port" in
[0-9]*)
	# Specific port found, forward it to the host gateway.
	sudo iptables -t nat -A PREROUTING -p tcp --dport "$port" -j DNAT --to-destination "$host_ip:$port"
	echo "Forwarded port $port to $host_ip"
	;;
*)
	# No specific port or non-numeric port, forward standard web ports.
	sudo iptables -t nat -A PREROUTING -p tcp --dport 80 -j DNAT --to-destination "$host_ip:80"
	sudo iptables -t nat -A PREROUTING -p tcp --dport 443 -j DNAT --to-destination "$host_ip:443"
	echo "Forwarded default ports 80/443 to $host_ip"
	;;
esac

# Start Docker service, which creates the "docker0" interface if it doesn't
# exist. We need the interface to extract the second IP address for DNS
# resolution.
sudo service docker start

# Configure DNS resolution to avoid requiring devcontainer project modifications.
# While devcontainers can use the "--add-host" flag, it requires explicit
# definition in devcontainer.json. Using a DNS server instead means every
# devcontainer project doesn't need to accommodate this.

# Wait for the workspace to acquire its Docker bridge IP address. The
# "hostname -I" command returns multiple IPs: the first is typically the host
# Docker bridge (172.17.0.0/16 range) and the second is the workspace Docker
# bridge (172.18.0.0/16). We need the second IP because that's where
# devcontainers will be able to reach us.
dns_ip=
while [ -z "$dns_ip" ]; do
	dns_ip=$(hostname -I | awk '{print $2}')
	if [ -z "$dns_ip" ]; then
		echo "Waiting for hostname -I to return a valid second IP address..."
		sleep 1
	fi
done

echo "Using DNS IP: $dns_ip"

# Install dnsmasq to provide custom DNS resolution. This lightweight DNS
# server allows us to override specific hostname lookups without affecting
# other DNS queries.
sudo apt-get update -y
sudo apt-get install -y dnsmasq

# Configure dnsmasq to resolve host.docker.internal to this workspace's IP.
# This ensures devcontainers can find the Coder server even when the "docker0"
# interface would normally shadow the hostname resolution.
echo "no-hosts" | sudo tee /etc/dnsmasq.conf
echo "address=/host.docker.internal/$dns_ip" | sudo tee -a /etc/dnsmasq.conf
echo "resolv-file=/etc/resolv.conf" | sudo tee -a /etc/dnsmasq.conf
echo "no-dhcp-interface=" | sudo tee -a /etc/dnsmasq.conf
echo "bind-interfaces" | sudo tee -a /etc/dnsmasq.conf
echo "listen-address=127.0.0.1,$dns_ip" | sudo tee -a /etc/dnsmasq.conf

sudo service dnsmasq restart

# Configure Docker daemon to use our custom DNS server. This is the critical
# piece that ensures all containers (including devcontainers) use our dnsmasq
# server for hostname resolution, allowing them to properly resolve
# host.docker.internal.
echo "{\"dns\": [\"$dns_ip\"]}" | sudo tee /etc/docker/daemon.json
sudo service docker restart
