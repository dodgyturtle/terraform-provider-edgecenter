package edgecenter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	edgecloud "github.com/Edge-Center/edgecentercloud-go"
	"github.com/Edge-Center/edgecentercloud-go/edgecenter/router/v1/routers"
	"github.com/Edge-Center/edgecentercloud-go/edgecenter/subnet/v1/subnets"
	"github.com/Edge-Center/edgecentercloud-go/edgecenter/task/v1/tasks"
)

const (
	RouterDeleting        int = 1200
	RouterCreatingTimeout int = 1200
	RouterPoint               = "routers"
)

func resourceRouter() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceRouterCreate,
		ReadContext:   resourceRouterRead,
		UpdateContext: resourceRouterUpdate,
		DeleteContext: resourceRouterDelete,
		Description:   "Represent router. Router enables you to dynamically exchange routes between networks",
		Importer: &schema.ResourceImporter{
			StateContext: func(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
				projectID, regionID, routerID, err := ImportStringParser(d.Id())
				if err != nil {
					return nil, err
				}
				d.Set("project_id", projectID)
				d.Set("region_id", regionID)
				d.SetId(routerID)

				return []*schema.ResourceData{d}, nil
			},
		},

		Schema: map[string]*schema.Schema{
			"project_id": {
				Type:         schema.TypeInt,
				Optional:     true,
				Description:  "The uuid of the project. Either 'project_id' or 'project_name' must be specified.",
				ExactlyOneOf: []string{"project_id", "project_name"},
			},
			"project_name": {
				Type:         schema.TypeString,
				Optional:     true,
				Description:  "The name of the project. Either 'project_id' or 'project_name' must be specified.",
				ExactlyOneOf: []string{"project_id", "project_name"},
			},
			"region_id": {
				Type:         schema.TypeInt,
				Optional:     true,
				Description:  "The uuid of the region. Either 'region_id' or 'region_name' must be specified.",
				ExactlyOneOf: []string{"region_id", "region_name"},
			},
			"region_name": {
				Type:         schema.TypeString,
				Optional:     true,
				Description:  "The name of the region. Either 'region_id' or 'region_name' must be specified.",
				ExactlyOneOf: []string{"region_id", "region_name"},
			},
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The name of the router.",
			},
			"external_gateway_info": {
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				MaxItems:    1,
				Description: "Information related to the external gateway.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"type": {
							Type:        schema.TypeString,
							Description: "Must be 'manual' or 'default'",
							Optional:    true,
							Computed:    true,
						},
						"enable_snat": {
							Type:     schema.TypeBool,
							Optional: true,
							Computed: true,
						},
						"network_id": {
							Type:        schema.TypeString,
							Description: "Id of the external network",
							Optional:    true,
							Computed:    true,
						},
						"external_fixed_ips": {
							Type:     schema.TypeList,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"ip_address": {
										Type:     schema.TypeString,
										Required: true,
									},
									"subnet_id": {
										Type:     schema.TypeString,
										Required: true,
									},
								},
							},
						},
					},
				},
			},
			"interfaces": {
				Type:        schema.TypeSet,
				Optional:    true,
				Set:         routerInterfaceUniqueID,
				Description: "Set of interfaces associated with the router.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"type": {
							Type:        schema.TypeString,
							Description: "must be 'subnet'",
							Required:    true,
						},
						"subnet_id": {
							Type:        schema.TypeString,
							Description: "Subnet for router interface must have a gateway IP",
							Required:    true,
						},
						"port_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"network_id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"mac_address": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"ip_address": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"routes": {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "List of static routes to be applied to the router.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"destination": {
							Type:     schema.TypeString,
							Required: true,
						},
						"nexthop": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "IPv4 address to forward traffic to if it's destination IP matches 'destination' CIDR",
						},
					},
				},
			},
			"last_updated": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "The timestamp of the last update (use with update context).",
			},
		},
	}
}

func resourceRouterCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	log.Println("[DEBUG] Start router creating")
	var diags diag.Diagnostics
	config := m.(*Config)
	provider := config.Provider

	client, err := CreateClient(provider, d, RouterPoint, VersionPointV1)
	if err != nil {
		return diag.FromErr(err)
	}

	createOpts := routers.CreateOpts{}

	createOpts.Name = d.Get("name").(string)

	egi := d.Get("external_gateway_info")
	if len(egi.([]interface{})) > 0 {
		gws, err := extractExternalGatewayInfoMap(egi.([]interface{}))
		if err != nil {
			return diag.FromErr(err)
		}
		createOpts.ExternalGatewayInfo = gws
	}

	ifs := d.Get("interfaces").(*schema.Set)
	if ifs.Len() > 0 {
		ifaces, err := extractInterfacesMap(ifs.List())
		if err != nil {
			return diag.FromErr(err)
		}
		createOpts.Interfaces = ifaces
	}

	rs := d.Get("routes")
	if len(rs.([]interface{})) > 0 {
		routes, err := extractHostRoutesMap(rs.([]interface{}))
		if err != nil {
			return diag.FromErr(err)
		}
		createOpts.Routes = routes
	}

	results, err := routers.Create(client, createOpts).Extract()
	if err != nil {
		return diag.FromErr(err)
	}

	taskID := results.Tasks[0]
	log.Printf("[DEBUG] Task id (%s)", taskID)
	routerID, err := tasks.WaitTaskAndReturnResult(client, taskID, true, RouterCreatingTimeout, func(task tasks.TaskID) (interface{}, error) {
		taskInfo, err := tasks.Get(client, string(task)).Extract()
		if err != nil {
			return nil, fmt.Errorf("cannot get task with ID: %s. Error: %w", task, err)
		}
		Router, err := routers.ExtractRouterIDFromTask(taskInfo)
		if err != nil {
			return nil, fmt.Errorf("cannot retrieve Router ID from task info: %w", err)
		}
		return Router, nil
	},
	)
	log.Printf("[DEBUG] Router id (%s)", routerID)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(routerID.(string))
	resourceRouterRead(ctx, d, m)

	log.Printf("[DEBUG] Finish router creating (%s)", routerID)

	return diags
}

func resourceRouterRead(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	log.Println("[DEBUG] Start router reading")
	log.Printf("[DEBUG] Start router reading%s", d.State())
	var diags diag.Diagnostics
	config := m.(*Config)
	provider := config.Provider
	routerID := d.Id()
	log.Printf("[DEBUG] Router id = %s", routerID)

	client, err := CreateClient(provider, d, RouterPoint, VersionPointV1)
	if err != nil {
		return diag.FromErr(err)
	}

	router, err := routers.Get(client, routerID).Extract()
	if err != nil {
		return diag.Errorf("cannot get router with ID: %s. Error: %s", routerID, err)
	}

	d.Set("name", router.Name)

	if len(router.ExternalGatewayInfo.ExternalFixedIPs) > 0 {
		egi := make(map[string]interface{}, 4)
		egilst := make([]map[string]interface{}, 1)
		egi["enable_snat"] = router.ExternalGatewayInfo.EnableSNat
		egi["network_id"] = router.ExternalGatewayInfo.NetworkID

		egist := d.Get("external_gateway_info")
		if len(egist.([]interface{})) > 0 {
			gws, err := extractExternalGatewayInfoMap(egist.([]interface{}))
			if err != nil {
				return diag.FromErr(err)
			}
			egi["type"] = gws.Type
		}

		efip := make([]map[string]string, len(router.ExternalGatewayInfo.ExternalFixedIPs))
		for i, fip := range router.ExternalGatewayInfo.ExternalFixedIPs {
			tmpfip := make(map[string]string, 1)
			tmpfip["ip_address"] = fip.IPAddress
			tmpfip["subnet_id"] = fip.SubnetID
			efip[i] = tmpfip
		}
		egi["external_fixed_ips"] = efip

		egilst[0] = egi
		d.Set("external_gateway_info", egilst)
	}

	ifs := make([]interface{}, 0, len(router.Interfaces))
	for _, iface := range router.Interfaces {
		for _, subnet := range iface.IPAssignments {
			smap := make(map[string]interface{}, 6)
			smap["port_id"] = iface.PortID
			smap["network_id"] = iface.NetworkID
			smap["mac_address"] = iface.MacAddress.String()
			smap["type"] = "subnet"
			smap["subnet_id"] = subnet.SubnetID
			smap["ip_address"] = subnet.IPAddress.String()
			ifs = append(ifs, smap)
		}
	}
	if err := d.Set("interfaces", schema.NewSet(routerInterfaceUniqueID, ifs)); err != nil {
		return diag.FromErr(err)
	}

	rs := make([]map[string]string, len(router.Routes))
	for i, r := range router.Routes {
		rmap := make(map[string]string, 2)
		rmap["destination"] = r.Destination.String()
		rmap["nexthop"] = r.NextHop.String()
		rs[i] = rmap
	}
	d.Set("routes", rs)

	log.Println("[DEBUG] Finish router reading")

	return diags
}

func resourceRouterUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	log.Println("[DEBUG] Start router updating")
	routerID := d.Id()
	log.Printf("[DEBUG] Router id = %s", routerID)
	config := m.(*Config)
	provider := config.Provider
	client, err := CreateClient(provider, d, RouterPoint, VersionPointV1)
	if err != nil {
		return diag.FromErr(err)
	}

	updateOpts := routers.UpdateOpts{}

	if d.HasChange("name") {
		updateOpts.Name = d.Get("name").(string)
	}

	// Only one kind of update is supported when external manual gateway is set.
	if d.HasChange("external_gateway_info") {
		egi := d.Get("external_gateway_info")
		if len(egi.([]interface{})) > 0 {
			gws, err := extractExternalGatewayInfoMap(egi.([]interface{}))
			if err != nil {
				return diag.FromErr(err)
			}
			if gws.Type == "manual" {
				updateOpts.ExternalGatewayInfo = gws
			}
		}
	}

	if d.HasChange("interfaces") {
		oldValue, newValue := d.GetChange("interfaces")
		oifs, err := extractInterfacesMap(oldValue.(*schema.Set).List())
		if err != nil {
			return diag.FromErr(err)
		}
		nifs, err := extractInterfacesMap(newValue.(*schema.Set).List())
		if err != nil {
			return diag.FromErr(err)
		}

		omap := make(map[string]int, len(oifs))

		for index, iface := range oifs {
			omap[iface.SubnetID] = index
		}

		for _, niface := range nifs {
			_, ok := omap[niface.SubnetID]
			if ok {
				delete(omap, niface.SubnetID)
			} else {
				_, err = routers.Attach(client, routerID, niface.SubnetID).Extract()
				if err != nil {
					return diag.FromErr(err)
				}
			}
		}

		for _, v := range omap {
			oiface := oifs[v]
			_, err = routers.Detach(client, routerID, oiface.SubnetID).Extract()
			if err != nil {
				return diag.FromErr(err)
			}
		}
	}

	if d.HasChange("routes") {
		rs := d.Get("routes")
		updateOpts.Routes = make([]subnets.HostRoute, 0)
		if len(rs.([]interface{})) > 0 {
			routes, err := extractHostRoutesMap(rs.([]interface{}))
			if err != nil {
				return diag.FromErr(err)
			}
			updateOpts.Routes = routes
		}
	}

	_, err = routers.Update(client, routerID, updateOpts).Extract()
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("last_updated", time.Now().Format(time.RFC850))
	log.Println("[DEBUG] Finish router updating")

	return resourceRouterRead(ctx, d, m)
}

func resourceRouterDelete(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	log.Println("[DEBUG] Start router deleting")
	var diags diag.Diagnostics
	config := m.(*Config)
	provider := config.Provider
	routerID := d.Id()
	log.Printf("[DEBUG] Router id = %s", routerID)

	client, err := CreateClient(provider, d, RouterPoint, VersionPointV1)
	if err != nil {
		return diag.FromErr(err)
	}

	results, err := routers.Delete(client, routerID).Extract()
	if err != nil {
		return diag.FromErr(err)
	}
	taskID := results.Tasks[0]
	log.Printf("[DEBUG] Task id (%s)", taskID)
	_, err = tasks.WaitTaskAndReturnResult(client, taskID, true, RouterDeleting, func(task tasks.TaskID) (interface{}, error) {
		_, err := routers.Get(client, routerID).Extract()
		if err == nil {
			return nil, fmt.Errorf("cannot delete router with ID: %s", routerID)
		}
		var errDefault404 edgecloud.Default404Error
		if errors.As(err, &errDefault404) {
			return nil, nil
		}
		return nil, fmt.Errorf("extracting Router resource error: %w", err)
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	log.Printf("[DEBUG] Finish of router deleting")

	return diags
}
