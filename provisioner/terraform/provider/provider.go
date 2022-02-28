package provider

import (
	"context"
	"net/url"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"

	"github.com/coder/coder/database"
	"github.com/coder/coder/provisionersdk"
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
				DefaultFunc: schema.EnvDefaultFunc("CODER_URL", ""),
				ValidateFunc: func(i interface{}, s string) ([]string, []error) {
					_, err := url.Parse(s)
					if err != nil {
						return nil, []error{err}
					}
					return nil, nil
				},
			},
		},
		ConfigureContextFunc: func(c context.Context, resourceData *schema.ResourceData) (interface{}, diag.Diagnostics) {
			rawURL, ok := resourceData.Get("url").(string)
			if !ok {
				return nil, diag.Errorf("unexpected type %q for url", reflect.TypeOf(resourceData.Get("url")).String())
			}
			if rawURL == "" {
				return nil, diag.Errorf("CODER_URL must not be empty; got %q", rawURL)
			}
			parsed, err := url.Parse(resourceData.Get("url").(string))
			if err != nil {
				return nil, diag.FromErr(err)
			}
			return config{
				URL: parsed,
			}, nil
		},
		DataSourcesMap: map[string]*schema.Resource{
			"coder_workspace": {
				Description: "TODO",
				ReadContext: func(c context.Context, rd *schema.ResourceData, i interface{}) diag.Diagnostics {
					rd.SetId(uuid.NewString())
					return nil
				},
				Schema: map[string]*schema.Schema{
					"transition": {
						Type:         schema.TypeString,
						Optional:     true,
						Description:  "TODO",
						DefaultFunc:  schema.EnvDefaultFunc("CODER_WORKSPACE_TRANSITION", ""),
						ValidateFunc: validation.StringInSlice([]string{string(database.WorkspaceTransitionStart), string(database.WorkspaceTransitionStop)}, false),
					},
				},
			},
			"coder_agent_script": {
				Description: "TODO",
				ReadContext: func(c context.Context, resourceData *schema.ResourceData, i interface{}) diag.Diagnostics {
					config, valid := i.(config)
					if !valid {
						return diag.Errorf("config was unexpected type %q", reflect.TypeOf(i).String())
					}
					operatingSystem, valid := resourceData.Get("os").(string)
					if !valid {
						return diag.Errorf("os was unexpected type %q", reflect.TypeOf(resourceData.Get("os")))
					}
					arch, valid := resourceData.Get("arch").(string)
					if !valid {
						return diag.Errorf("arch was unexpected type %q", reflect.TypeOf(resourceData.Get("arch")))
					}
					script, err := provisionersdk.AgentScript(config.URL, operatingSystem, arch)
					if err != nil {
						return diag.FromErr(err)
					}
					err = resourceData.Set("value", script)
					if err != nil {
						return diag.FromErr(err)
					}
					resourceData.SetId(strings.Join([]string{operatingSystem, arch}, "_"))
					return nil
				},
				Schema: map[string]*schema.Schema{
					"os": {
						Type:         schema.TypeString,
						Required:     true,
						ValidateFunc: validation.StringInSlice([]string{"linux", "darwin", "windows"}, false),
					},
					"arch": {
						Type:         schema.TypeString,
						Required:     true,
						ValidateFunc: validation.StringInSlice([]string{"amd64"}, false),
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
					err := rd.Set("token", uuid.NewString())
					if err != nil {
						return diag.FromErr(err)
					}
					return nil
				},
				ReadContext: func(c context.Context, rd *schema.ResourceData, i interface{}) diag.Diagnostics {
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
									ForceNew:     true,
									Description:  "TODO",
									Optional:     true,
									Type:         schema.TypeString,
									ValidateFunc: validation.StringInSlice([]string{"google-instance-identity"}, false),
								},
								"instance_id": {
									ForceNew:    true,
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
						ForceNew: true,
						Type:     schema.TypeString,
						Computed: true,
					},
				},
			},
		},
	}
}
