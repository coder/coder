package terraform_provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider -
func CoderProvider() *schema.Provider {
	p := &schema.Provider{
		Schema: map[string]*schema.Schema{
			// The URL of the actual coder instance - we communicate back to it from the provider to set parameters.
			"coder_host_url": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("CODER_AGENT_HOST_URL", nil),
			},
			"coder_agent_additional_args": {
				Type:        schema.TypeString,
				Required:    false,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("CODER_AGENT_ADDITIONAL_ARGS", ""),
			},
			"coder_agent_environment_variable": &schema.Schema{
				Type:     schema.TypeList,
				Required: false,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"environment_variable": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
						"value": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
		},
		ResourcesMap: map[string]*schema.Resource{},
		DataSourcesMap: map[string]*schema.Resource{
			"coder_agent": dataSourceAgentScript(),
		},
	}

	p.ConfigureFunc = providerConfigure

	return p
}

func providerConfigure(d *schema.ResourceData) (interface{}, error) {

	out := Config{}

	if v, ok := d.GetOk("coder_agent_additional_args"); ok {
		out.AdditionalArgs = v.(string)
	}

	if env, ok := d.GetOk("coder_agent_environment_variable"); ok {
		envArray := env.([]interface{})
		outputVariables := []EnvironmentVariable{}

		for _, item := range envArray {
			itemCastToMap := item.(map[string]interface{})
			variableName, ok := itemCastToMap["environment_variable"]
			if !ok {
				continue
			}

			variableValue, ok := itemCastToMap["value"]
			if !ok {
				continue
			}

			outputVariables = append(outputVariables, EnvironmentVariable{
				Name:  fmt.Sprintf("%s", variableName),
				Value: fmt.Sprintf("%s", variableValue),
			})
		}

		out.EnvironmentVariables = outputVariables
	}

	return out, nil
}
