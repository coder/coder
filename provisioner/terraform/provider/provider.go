package provider

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// New returns a new schema provider for Terraform.
func New() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"workspace_history_id": {
				Type:     schema.TypeString,
				Optional: true,
			},
		},
		DataSourcesMap: map[string]*schema.Resource{
			"coder_agent_script": {
				Description: "TODO",
				ReadContext: func(c context.Context, rd *schema.ResourceData, i interface{}) diag.Diagnostics {
					osRaw := rd.Get("os")
					os := osRaw.(string)

					archRaw := rd.Get("arch")
					arch := archRaw.(string)

					fmt.Printf("Got OS: %s_%s\n", os, arch)

					err := rd.Set("value", "SOME SCRIPT")
					if err != nil {
						return diag.FromErr(err)
					}
					rd.SetId("something")
					return nil
				},
				Schema: map[string]*schema.Schema{
					"os": {
						Type:         schema.TypeString,
						Required:     true,
						ValidateFunc: validation.StringInSlice([]string{"linux", "darwin", "windows"}, false),
					},
					"arch": {
						Type:     schema.TypeString,
						Required: true,
					},
					"value": {
						Type:     schema.TypeString,
						Computed: true,
					},
				},
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"coder_agent": {
				Description: "TODO",
				CreateContext: func(c context.Context, rd *schema.ResourceData, i interface{}) diag.Diagnostics {
					// This should be a real authentication token!
					rd.SetId(uuid.NewString())
					rd.Set("token", uuid.NewString())
					return nil
				},
				ReadContext: func(c context.Context, rd *schema.ResourceData, i interface{}) diag.Diagnostics {
					authRaw := rd.Get("auth").([]interface{})[0]
					if authRaw != nil {
						auth := authRaw.(map[string]interface{})
						fmt.Printf("Auth got %+v\n", auth)
					}

					env := rd.Get("env").(map[string]interface{})
					for key, value := range env {
						fmt.Printf("Got: %s, %s\n", key, value)
					}
					return nil
				},
				DeleteContext: func(c context.Context, rd *schema.ResourceData, i interface{}) diag.Diagnostics {
					return nil
				},
				Schema: map[string]*schema.Schema{
					"auth": {
						ForceNew:    true,
						Description: "TODO",
						Type:        schema.TypeList,
						Optional:    true,
						MaxItems:    1,
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"type": {
									Description: "TODO",
									Optional:    true,
									Type:        schema.TypeString,
								},
								"instance_id": {
									Description: "TODO",
									Optional:    true,
									Type:        schema.TypeString,
								},
							},
						},
					},
					"env": {
						ForceNew:    true,
						Description: "TODO",
						Type:        schema.TypeMap,
						Optional:    true,
					},
					"startup_script": {
						ForceNew:    true,
						Description: "TODO",
						Type:        schema.TypeString,
						Optional:    true,
					},
					"token": {
						Type:     schema.TypeString,
						Computed: true,
					},
				},
			},
		},
	}
}
