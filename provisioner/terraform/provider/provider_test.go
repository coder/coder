package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/provisioner/terraform/provider"
)

func TestProvider(t *testing.T) {
	t.Parallel()
	tfProvider := provider.New()
	err := tfProvider.InternalValidate()
	require.NoError(t, err)
}

func TestAgentScript(t *testing.T) {
	t.Parallel()
	resource.Test(t, resource.TestCase{
		Providers: map[string]*schema.Provider{
			"coder": provider.New(),
		},
		IsUnitTest: true,
		Steps: []resource.TestStep{{
			Config: `
			provider "coder" {
				url = "https://example.com"
			}
			data "coder_agent_script" "new" {
				arch = "amd64"
				os = "linux"
			}`,
			Check: func(state *terraform.State) error {
				require.Len(t, state.Modules, 1)
				require.Len(t, state.Modules[0].Resources, 1)
				resource := state.Modules[0].Resources["data.coder_agent_script.new"]
				require.NotNil(t, resource)
				value := resource.Primary.Attributes["value"]
				require.NotNil(t, value)
				t.Log(value)
				return nil
			},
		}},
	})
}

func TestAgent(t *testing.T) {
	t.Parallel()
	resource.Test(t, resource.TestCase{
		Providers: map[string]*schema.Provider{
			"coder": provider.New(),
		},
		IsUnitTest: true,
		Steps: []resource.TestStep{{
			Config: `
			provider "coder" {
				url = "https://example.com"
			}
			resource "coder_agent" "new" {
				auth {
					type = "something"
					instance_id = "instance"
				}
				env = {
					hi = "test"
				}
				startup_script = "echo test"
			}`,
			Check: func(state *terraform.State) error {
				require.Len(t, state.Modules, 1)
				require.Len(t, state.Modules[0].Resources, 1)
				resource := state.Modules[0].Resources["coder_agent.new"]
				require.NotNil(t, resource)
				for _, k := range []string{
					"token",
					"auth.0.type",
					"auth.0.instance_id",
					"env.hi",
					"startup_script",
				} {
					v := resource.Primary.Attributes[k]
					t.Log(fmt.Sprintf("%q = %q", k, v))
					require.NotNil(t, v)
					require.Greater(t, len(v), 0)
				}
				return nil
			},
		}},
	})
}
