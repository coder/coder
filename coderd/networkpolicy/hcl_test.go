package networkpolicy_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/networkpolicy"
)

func TestValidateHCLInputSecurity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		input string
	}{
		{"numeric_interpolation", `allowed_https_hosts = ["host${1e1000}.com"]`},
		{"boolean_interpolation", `allowed_https_hosts = ["host${true}.com"]`},
		{"template_loop", `allowed_https_hosts = ["%{ for host in ["github.com"] }${host}%{ endfor }"]`},
		{"conditional", `direct_p2p_enabled = true ? false : true`},
		{"computed_object_key", `relay_allowances = [{ ("cidrs") = ["192.0.2.1/32"], ports = [{ port = 443 }] }]`},
		{"interpolated_object_key", `relay_allowances = [{ "cidrs${true}" = [], ports = [{ port = 443 }] }]`},
		{"duplicate_decoded_key", `relay_allowances = [{ cidrs = ["192.0.2.1/32"], "\u0063idrs" = ["0.0.0.0/0"], ports = [{ port = 443 }] }]`},
		{"object_comprehension", `relay_allowances = [{ for key in ["cidrs"] : key => [] }]`},
		{"null_input", `allowed_https_hosts = null`},
		{"null_host", `allowed_https_hosts = [null]`},
		{"numeric_host", `allowed_https_hosts = [443]`},
		{"null_allowance", `relay_allowances = [null]`},
		{"list_allowance", `relay_allowances = [[]]`},
		{"null_cidrs", `relay_allowances = [{ cidrs = null, ports = [] }]`},
		{"null_selectors", `relay_allowances = [{ endpoint_selectors = null, ports = [] }]`},
		{"null_selector", `relay_allowances = [{ endpoint_selectors = [null], ports = [] }]`},
		{"null_label", `relay_allowances = [{ endpoint_selectors = [{ app = null }], ports = [] }]`},
		{"boolean_label", `relay_allowances = [{ endpoint_selectors = [{ app = true }], ports = [] }]`},
		{"missing_ports", `relay_allowances = [{ cidrs = ["192.0.2.1/32"] }]`},
		{"null_ports", `relay_allowances = [{ ports = null }]`},
		{"null_port", `relay_allowances = [{ ports = [null] }]`},
		{"missing_port_number", `relay_allowances = [{ ports = [{ protocol = "TCP" }] }]`},
		{"string_port_number", `relay_allowances = [{ ports = [{ port = "443" }] }]`},
		{"null_protocol", `relay_allowances = [{ ports = [{ port = 443, protocol = null }] }]`},
		{"numeric_protocol", `relay_allowances = [{ ports = [{ port = 443, protocol = 6 }] }]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := "module \"coder_network_policy\" {\n source = \"./module\"\n " + tc.input + "\n}"
			require.Error(t, networkpolicy.ValidateHCL(source))
			archive, err := networkpolicy.Archive(source, fstest.MapFS{
				"main.tf": &fstest.MapFile{Data: []byte("# trusted module\n")},
			})
			require.Error(t, err)
			require.Empty(t, archive, "invalid inputs must never reach Terraform")
		})
	}
}

func TestValidateHCLLimits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		source string
		want   string
	}{
		{"bytes", strings.Repeat(" ", 1<<20+1), "exceeds 1 MiB"},
		{"tokens", `module "coder_network_policy" { allowed_https_hosts = [` + strings.Repeat(`"github.com",`, 8192) + `] }`, "exceeds 16384 tokens"},
		{"depth", `module "coder_network_policy" { allowed_https_hosts = ` + strings.Repeat("[", 33) + `"github.com"` + strings.Repeat("]", 33) + ` }`, "nesting exceeds 32 levels"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorContains(t, networkpolicy.ValidateHCL(tc.source), tc.want)
		})
	}
}

func TestValidateHCL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		hcl   string
		valid bool
	}{
		{"minimal", `module "coder_network_policy" { source = "./module" }`, true},
		{"static_inputs", `module "coder_network_policy" {
 source = "./module"
 allowed_https_hosts = ["github.com", "proxy.golang.org"]
 direct_p2p_enabled = false
}`, true},
		{"comprehension", `module "coder_network_policy" {
 source = "./module"
 allowed_https_hosts = [for x in [1,2,3] : [for y in [1,2,3] : "github.com"]]
}`, false},
		{"unknown_port_field", `module "coder_network_policy" {
 source = "./module"
 control_plane_allowances = [{cidrs = ["0.0.0.0/0"], ports = [{port = 443, serverNames = ["github.com"]}]}]
}`, false},
		{"unknown_allowance_field", `module "coder_network_policy" {
 source = "./module"
 relay_allowances = [{cidrs = ["0.0.0.0/0"], ports = [{port = 443}], serverNames = ["github.com"]}]
}`, false},
		{"explicit_dependency", `module "coder_network_policy" {
 source = "./module"
 control_plane_allowances = [{endpoint_selectors = [{"k8s:io.kubernetes.pod.namespace" = "coder", "k8s:app" = "coder"}], ports = [{port = 8080, protocol = "TCP"}]}]
}`, true},
		{"wrong_bool_type", `module "coder_network_policy" {
 source = "./module"
 direct_p2p_enabled = "false"
}`, false},
		{"excessive_nesting", `module "coder_network_policy" { allowed_https_hosts = ` + strings.Repeat("[", 100) + `"github.com"` + strings.Repeat("]", 100) + ` }`, false},
		{"duplicate_key", `module "coder_network_policy" {
 source = "./module"
 relay_allowances = [{cidrs = ["192.0.2.1/32"], cidrs = ["0.0.0.0/0"], ports = [{port = 443}]}]
}`, false},
		{"no_module", ``, false},
		{"wrong_module_name", `module "network_policy" { source = "./module" }`, false},
		{"root_resource", `resource "null_resource" "exec" {}`, false},
		{"external_source", `module "coder_network_policy" { source = "https://example.com/module.zip" }`, false},
		{"identity_override", `module "coder_network_policy" {
 source = "./module"
 template_ids = ["other-template"]
}`, false},
		{"function", `module "coder_network_policy" {
 source = "./module"
 allowed_https_hosts = [file("/etc/passwd")]
}`, false},
		{"variable", `module "coder_network_policy" {
 source = "./module"
 allowed_https_hosts = var.hosts
}`, false},
		{"providers", `module "coder_network_policy" {
 source = "./module"
 providers = { kubernetes = kubernetes.other }
}`, false},
		{"unknown_input", `module "coder_network_policy" {
 source = "./module"
 arbitrary = true
}`, false},
		{"nested_block", `module "coder_network_policy" {
 source = "./module"
 provisioner "local-exec" { command = "true" }
}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := networkpolicy.ValidateHCL(tc.hcl)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
