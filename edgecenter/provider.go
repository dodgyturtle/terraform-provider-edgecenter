package edgecenter

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/Edge-Center/terraform-provider-edgecenter/edgecenter/config"
	"github.com/Edge-Center/terraform-provider-edgecenter/edgecenter/floatingip"
	"github.com/Edge-Center/terraform-provider-edgecenter/edgecenter/volume"
)

// Provider returns a schema.Provider for Edgecenter.
func Provider() *schema.Provider {
	p := &schema.Provider{
		Schema: map[string]*schema.Schema{
			"api_key": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{"EC_PERMANENT_TOKEN", "API_KEY"}, nil),
				Sensitive:   true,
				Description: "A permanent [API-token](https://support.edgecenter.ru/knowledge_base/item/257788)",
			},
			"edgecenter_cloud_api": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Region API (define only if you want to override Region API endpoint)",
				DefaultFunc: schema.EnvDefaultFunc("EC_CLOUD_API", nil),
			},
		},
		DataSourcesMap: map[string]*schema.Resource{
			"edgecenter_floatingip": floatingip.DataSourceEdgeCenterFloatingIP(),
			"edgecenter_volume":     volume.DataSourceEdgeCenterVolume(),
		},
		ResourcesMap: map[string]*schema.Resource{
			"edgecenter_floatingip": floatingip.ResourceEdgeCenterFloatingIP(),
			"edgecenter_volume":     volume.ResourceEdgeCenterVolume(),
		},
	}

	p.ConfigureContextFunc = func(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
		terraformVersion := p.TerraformVersion
		if terraformVersion == "" {
			terraformVersion = "0.12+compatible"
		}
		return providerConfigure(ctx, d, terraformVersion)
	}

	return p
}

func providerConfigure(_ context.Context, d *schema.ResourceData, terraformVersion string) (interface{}, diag.Diagnostics) {
	conf := config.Config{
		TerraformVersion: terraformVersion,
		APIKey:           d.Get("api_key").(string),
		CloudAPIURL:      d.Get("edgecenter_cloud_api").(string),
	}

	return conf.Client()
}
