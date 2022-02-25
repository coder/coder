package provider

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/coder/coder/provisionersdk"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

type config struct {
	URL *url.URL
}

// New returns a new Terraform provider.
func New() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"url": {
				Type:     schema.TypeString,
				Optional: true,
				// The "CODER_URL" environment variable is used by default
				// as the Access URL when generating scripts.
				DefaultFunc: func() (interface{}, error) {
					return os.Getenv("CODER_URL"), nil
				},
				ValidateFunc: func(i interface{}, s string) ([]string, []error) {
					_, err := url.Parse(s)
					if err != nil {
						return nil, []error{err}
					}
					return nil, nil
				},
			},
		},
		ConfigureContextFunc: func(c context.Context, rd *schema.ResourceData) (interface{}, diag.Diagnostics) {
			rawURL := rd.Get("url").(string)
			if rawURL == "" {
				return nil, diag.Errorf("CODER_URL must not be empty; got %q", rawURL)
			}
			parsed, err := url.Parse(rd.Get("url").(string))
			if err != nil {
				return nil, diag.FromErr(err)
			}
			return config{
				URL: parsed,
			}, nil
		},
		DataSourcesMap: map[string]*schema.Resource{
			"coder_agent_script": {
				Description: "TODO",
				ReadContext: func(c context.Context, rd *schema.ResourceData, i interface{}) diag.Diagnostics {
					config := i.(config)
					osRaw := rd.Get("os")
					os := osRaw.(string)
					archRaw := rd.Get("arch")
					arch := archRaw.(string)

					script, err := provisionersdk.AgentScript(os, arch, config.URL)
					if err != nil {
						return diag.FromErr(err)
					}
					err = rd.Set("value", script)
					if err != nil {
						return diag.FromErr(err)
					}
					rd.SetId(strings.Join([]string{os, arch}, "_"))
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
