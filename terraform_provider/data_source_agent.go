package terraform_provider

import (
	"context"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type coffee struct {
	id   int
	name string
}

func dataSourceAgentScript() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceCoffeesRead,
		Schema: map[string]*schema.Schema{
			"linux": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceCoffeesRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	if err := d.Set("linux", "coder agent run"); err != nil {
		return diag.FromErr(err)
	}

	// always run
	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))

	return diags
}
