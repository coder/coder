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

func TestSomething(t *testing.T) {
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
			Check: func(s *terraform.State) error {
				fmt.Printf("check state: %+v\n", s)
				return nil
			},
		}},
	})
}

func TestAnother(t *testing.T) {
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
					type = "gcp"
				}
				env = {
					test = "some magic value"
				}
			}`,
			Check: func(s *terraform.State) error {
				fmt.Printf("State: %+v\n", s)
				// for _, mod := range s.Modules {
				// 	fmt.Printf("check state: %+v\n", mod.Resources)
				// }
				// data, _ := json.MarshalIndent(s, "", "\t")
				// fmt.Printf("Data: %s\n", data)

				return nil
			},
		}},
	})
}
