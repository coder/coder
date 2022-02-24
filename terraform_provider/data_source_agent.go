package terraform_provider

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceAgentScript() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceReadScripts,
		Schema: map[string]*schema.Schema{
			"linux": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceReadScripts(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	additionalArgs := m
	if additionalArgs == nil {
		additionalArgs = ""
	}

	if err := d.Set("linux", fmt.Sprintf("coder agent run %s", additionalArgs)); err != nil {
		return diag.FromErr(err)
	}

	// always run
	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))

	return diags
}
