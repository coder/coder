#!/bin/sh
set -e

if [ "${CODER_AGENT_URL#*host.docker.internal}" = "$CODER_AGENT_URL" ]; then
	# This is likely an external access URL, so we do not need to set up
	# port forwarding or DNS resolution for host.docker.internal.

	# Start the docker service if it is not running.
	sudo service docker start

	exit 0
fi

# The access URL is host.docker.internal, so we must set up forwarding
# to the host Docker gateway IP address, which is typically 172.17.0.1,
# this will allow the devcontainers to access the Coder server even if
# the access URL has been shadowed by a "docker0" interface. This
# usually happens if docker is started inside a devcontainer.

# Enable IP forwarding to allow traffic to flow between the host and
# the devcontainers.
echo 1 | sudo tee /proc/sys/net/ipv4/ip_forward
sudo iptables -t nat -A POSTROUTING -j MASQUERADE

# Get the IP address of the host Docker gateway, which is
# typically 172.17.0.1 and set up port forwarding between this
# workspace's Docker gateway and the host Docker gateway.
host_ip=$(getent hosts host.docker.internal | awk '{print $1}')
port="${CODER_AGENT_URL##*:}"
port="${port%%/*}"
case "$port" in
[0-9]*)
	sudo iptables -t nat -A PREROUTING -p tcp --dport $port -j DNAT --to-destination $host_ip:$port
	echo "Forwarded port $port to $host_ip"
	;;
*)
	sudo iptables -t nat -A PREROUTING -p tcp --dport 80 -j DNAT --to-destination $host_ip:80
	sudo iptables -t nat -A PREROUTING -p tcp --dport 443 -j DNAT --to-destination $host_ip:443
	echo "Forwarded default ports 80/443 to $host_ip"
	;;
esac

# Start the docker service if it is not running, this will create
# the "docker0" interface if it does not exist.
sudo service docker start

# Since we cannot define "--add-host" for devcontainers, we define
# a dnsmasq configuration that allows devcontainers to resolve the
# host.docker.internal URL to this workspace, which is typically
# 172.18.0.1. Note that we take the second IP address from
# "hostname -I" because the first one is usually in the range
# 172.17.0.0/16, which is the host Docker bridge.
dns_ip=
while [ -z "$dns_ip" ]; do
	dns_ip=$(hostname -I | awk '{print $2}')
	if [ -z "$dns_ip" ]; then
		echo "Waiting for hostname -I to return a valid second IP address..."
		sleep 1
	fi
done

# Create a simple dnsmasq configuration to allow devcontainers to
# resolve host.docker.internal.
sudo apt-get update -y
sudo apt-get install -y dnsmasq

echo "no-hosts" | sudo tee /etc/dnsmasq.conf
echo "address=/host.docker.internal/$dns_ip" | sudo tee -a /etc/dnsmasq.conf
echo "resolv-file=/etc/resolv.conf" | sudo tee -a /etc/dnsmasq.conf
echo "no-dhcp-interface=" | sudo tee -a /etc/dnsmasq.conf
echo "bind-interfaces" | sudo tee -a /etc/dnsmasq.conf
echo "listen-address=127.0.0.1,$dns_ip" | sudo tee -a /etc/dnsmasq.conf

# Restart dnsmasq to apply the new configuration.
sudo service dnsmasq restart

# Configure Docker to use the dnsmasq server for DNS resolution.
# This allows devcontainers to resolve host.docker.internal to the
# IP address of this workspace.
echo "{\"dns\": [\"$dns_ip\"]}" | sudo tee /etc/docker/daemon.json

# Restart the Docker service to apply the new configuration.
sudo service docker restart
