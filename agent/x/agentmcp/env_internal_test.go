package agentmcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterPluginEnv(t *testing.T) {
	t.Parallel()

	in := []string{
		"PATH=/usr/bin",
		"HOME=/home/u",
		"CODER_AGENT_TOKEN=secret",
		"CODER_AGENT_URL=https://coder.example.com",
		"LC_ALL=C.UTF-8",
		"LC_TIME=en_US.UTF-8",
		"XDG_CONFIG_HOME=/home/u/.config",
		"LD_PRELOAD=/evil.so",
		"GITHUB_TOKEN=ghp",
		"https_proxy=http://proxy:3128",
		"NO_PROXY=localhost",
		"SSL_CERT_FILE=/etc/ssl/ca.pem",
		"NODE_EXTRA_CA_CERTS=/etc/ssl/ca.pem",
		"TMPDIR=/tmp",
		"NOEQUALS",
		"LCX=not-a-prefix-match",
		"XDGSOMETHING=1",
	}
	want := []string{
		"PATH=/usr/bin",
		"HOME=/home/u",
		"LC_ALL=C.UTF-8",
		"LC_TIME=en_US.UTF-8",
		"XDG_CONFIG_HOME=/home/u/.config",
		"https_proxy=http://proxy:3128",
		"NO_PROXY=localhost",
		"SSL_CERT_FILE=/etc/ssl/ca.pem",
		"NODE_EXTRA_CA_CERTS=/etc/ssl/ca.pem",
		"TMPDIR=/tmp",
	}
	assert.Equal(t, want, filterPluginEnv(in))
}
