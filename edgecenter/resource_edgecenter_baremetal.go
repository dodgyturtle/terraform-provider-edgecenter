package edgecenter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	edgecloud "github.com/Edge-Center/edgecentercloud-go"
	"github.com/Edge-Center/edgecentercloud-go/edgecenter/baremetal/v1/bminstances"
	"github.com/Edge-Center/edgecentercloud-go/edgecenter/instance/v1/instances"
	"github.com/Edge-Center/edgecentercloud-go/edgecenter/instance/v1/types"
	"github.com/Edge-Center/edgecentercloud-go/edgecenter/task/v1/tasks"
)

const (
	BmInstanceDeleting        int = 1200
	BmInstanceCreatingTimeout int = 3600
	BmInstancePoint               = "bminstances"
)

var bmCreateTimeout = time.Second * time.Duration(BmInstanceCreatingTimeout)

func resourceBmInstance() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceBmInstanceCreate,
		ReadContext:   resourceBmInstanceRead,
		UpdateContext: resourceBmInstanceUpdate,
		DeleteContext: resourceBmInstanceDelete,
		Description:   "Represent baremetal instance",
		Timeouts: &schema.ResourceTimeout{
			Create: &bmCreateTimeout,
		},
		Importer: &schema.ResourceImporter{
			StateContext: func(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
				projectID, regionID, InstanceID, err := ImportStringParser(d.Id())
				if err != nil {
					return nil, err
				}
				d.Set("project_id", projectID)
				d.Set("region_id", regionID)
				d.SetId(InstanceID)

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
			"flavor_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			"interface": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"type": {
							Type:        schema.TypeString,
							Required:    true,
							Description: fmt.Sprintf("Available value is '%s', '%s', '%s', '%s'", types.SubnetInterfaceType, types.AnySubnetInterfaceType, types.ExternalInterfaceType, types.ReservedFixedIPType),
						},
						"is_parent": {
							Type:        schema.TypeBool,
							Computed:    true,
							Optional:    true,
							Description: "If not set will be calculated after creation. Trunk interface always attached first. Can't detach interface if is_parent true. Fields affect only on creation",
						},
						"order": {
							Type:        schema.TypeInt,
							Optional:    true,
							Description: "Order of attaching interface. Trunk interface always attached first, fields affect only on creation",
						},
						"network_id": {
							Type:        schema.TypeString,
							Description: "required if type is 'subnet' or 'any_subnet'",
							Optional:    true,
							Computed:    true,
						},
						"subnet_id": {
							Type:        schema.TypeString,
							Description: "required if type is 'subnet'",
							Optional:    true,
							Computed:    true,
						},
						"port_id": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "required if type is  'reserved_fixed_ip'",
							Optional:    true,
						},
						// nested map is not supported, in this case, you do not need to use the list for the map
						"fip_source": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"existing_fip_id": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"ip_address": {
							Type:     schema.TypeString,
							Computed: true,
							Optional: true,
						},
					},
				},
			},
			"name": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "The name of the baremetal instance.",
			},
			"name_templates": {
				Type:          schema.TypeList,
				Optional:      true,
				Deprecated:    "Use name_template instead",
				ConflictsWith: []string{"name_template"},
				Elem:          &schema.Schema{Type: schema.TypeString},
			},
			"name_template": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"name_templates"},
			},
			"image_id": {
				Type:     schema.TypeString,
				Optional: true,
				ExactlyOneOf: []string{
					"image_id",
					"apptemplate_id",
				},
			},
			"apptemplate_id": {
				Type:     schema.TypeString,
				Optional: true,
				ExactlyOneOf: []string{
					"image_id",
					"apptemplate_id",
				},
			},
			"keypair_name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"password": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"username": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"metadata": {
				Type:          schema.TypeList,
				Optional:      true,
				Deprecated:    "Use metadata_map instead",
				ConflictsWith: []string{"metadata_map"},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"key": {
							Type:     schema.TypeString,
							Required: true,
						},
						"value": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"metadata_map": {
				Type:          schema.TypeMap,
				Optional:      true,
				ConflictsWith: []string{"metadata"},
				Description:   "A map containing metadata, for example tags.",
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"app_config": {
				Type:     schema.TypeMap,
				Optional: true,
			},
			"user_data": {
				Type:     schema.TypeString,
				Optional: true,
			},

			// computed
			"flavor": {
				Type:     schema.TypeMap,
				Computed: true,
			},
			"status": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"vm_state": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"addresses": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"net": {
							Type:     schema.TypeList,
							Required: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"addr": {
										Type:     schema.TypeString,
										Required: true,
									},
									"type": {
										Type:     schema.TypeString,
										Required: true,
									},
								},
							},
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

func resourceBmInstanceCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	log.Println("[DEBUG] Start BaremetalInstance creating")
	var diags diag.Diagnostics
	config := m.(*Config)
	provider := config.Provider

	client, err := CreateClient(provider, d, BmInstancePoint, VersionPointV1)
	if err != nil {
		return diag.FromErr(err)
	}

	ifs := d.Get("interface").([]interface{})
	// sort interfaces by 'is_parent' at first and by 'order' key to attach it in right order
	sort.Sort(instanceInterfaces(ifs))
	interfaceOptsList := make([]bminstances.InterfaceOpts, len(ifs))
	for i, iFace := range ifs {
		raw := iFace.(map[string]interface{})
		interfaceOpts := bminstances.InterfaceOpts{
			Type:      types.InterfaceType(raw["type"].(string)),
			NetworkID: raw["network_id"].(string),
			SubnetID:  raw["subnet_id"].(string),
			PortID:    raw["port_id"].(string),
		}

		fipSource := raw["fip_source"].(string)
		fipID := raw["existing_fip_id"].(string)
		if fipSource != "" {
			interfaceOpts.FloatingIP = &bminstances.CreateNewInterfaceFloatingIPOpts{
				Source:             types.FloatingIPSource(fipSource),
				ExistingFloatingID: fipID,
			}
		}
		interfaceOptsList[i] = interfaceOpts
	}

	log.Printf("[DEBUG] Baremetal interfaces: %+v", interfaceOptsList)
	opts := bminstances.CreateOpts{
		Flavor:        d.Get("flavor_id").(string),
		ImageID:       d.Get("image_id").(string),
		AppTemplateID: d.Get("apptemplate_id").(string),
		Keypair:       d.Get("keypair_name").(string),
		Password:      d.Get("password").(string),
		Username:      d.Get("username").(string),
		UserData:      d.Get("user_data").(string),
		AppConfig:     d.Get("app_config").(map[string]interface{}),
		Interfaces:    interfaceOptsList,
	}

	name := d.Get("name").(string)
	if len(name) > 0 {
		opts.Names = []string{name}
	}

	if nameTemplatesRaw, ok := d.GetOk("name_templates"); ok {
		nameTemplates := nameTemplatesRaw.([]interface{})
		if len(nameTemplates) > 0 {
			NameTemp := make([]string, len(nameTemplates))
			for i, nametemp := range nameTemplates {
				NameTemp[i] = nametemp.(string)
			}
			opts.NameTemplates = NameTemp
		}
	} else if nameTemplate, ok := d.GetOk("name_template"); ok {
		opts.NameTemplates = []string{nameTemplate.(string)}
	}

	if metadata, ok := d.GetOk("metadata"); ok {
		if len(metadata.([]interface{})) > 0 {
			md, err := extractKeyValue(metadata.([]interface{}))
			if err != nil {
				return diag.FromErr(err)
			}
			opts.Metadata = &md
		}
	} else if metadataRaw, ok := d.GetOk("metadata_map"); ok {
		md := extractMetadataMap(metadataRaw.(map[string]interface{}))
		opts.Metadata = &md
	}

	results, err := bminstances.Create(client, opts).Extract()
	if err != nil {
		return diag.FromErr(err)
	}

	taskID := results.Tasks[0]

	InstanceID, err := tasks.WaitTaskAndReturnResult(client, taskID, true, BmInstanceCreatingTimeout, func(task tasks.TaskID) (interface{}, error) {
		taskInfo, err := tasks.Get(client, string(task)).Extract()
		if err != nil {
			return nil, fmt.Errorf("cannot get task with ID: %s. Error: %w", task, err)
		}
		Instance, err := instances.ExtractInstanceIDFromTask(taskInfo)
		if err != nil {
			return nil, fmt.Errorf("cannot retrieve Instance ID from task info: %w", err)
		}
		return Instance, nil
	},
	)
	log.Printf("[DEBUG] Baremetal Instance id (%s)", InstanceID)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(InstanceID.(string))
	resourceBmInstanceRead(ctx, d, m)

	log.Printf("[DEBUG] Finish Baremetal Instance creating (%s)", InstanceID)

	return diags
}

func resourceBmInstanceRead(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	log.Println("[DEBUG] Start Baremetal Instance reading")
	var diags diag.Diagnostics
	config := m.(*Config)
	provider := config.Provider
	instanceID := d.Id()
	log.Printf("[DEBUG] Instance id = %s", instanceID)

	client, err := CreateClient(provider, d, InstancePoint, VersionPointV1)
	if err != nil {
		return diag.FromErr(err)
	}

	instance, err := instances.Get(client, instanceID).Extract()
	if err != nil {
		return diag.Errorf("cannot get instance with ID: %s. Error: %s", instanceID, err)
	}

	d.Set("name", instance.Name)
	d.Set("flavor_id", instance.Flavor.FlavorID)
	d.Set("status", instance.Status)
	d.Set("vm_state", instance.VMState)

	flavor := make(map[string]interface{}, 4)
	flavor["flavor_id"] = instance.Flavor.FlavorID
	flavor["flavor_name"] = instance.Flavor.FlavorName
	flavor["ram"] = strconv.Itoa(instance.Flavor.RAM)
	flavor["vcpus"] = strconv.Itoa(instance.Flavor.VCPUS)
	d.Set("flavor", flavor)

	interfacesListAPI, err := instances.ListInterfacesAll(client, instanceID)
	if err != nil {
		return diag.FromErr(err)
	}

	if len(interfacesListAPI) == 0 {
		return diag.Errorf("interface not found")
	}

	ifs := d.Get("interface").([]interface{})
	sort.Sort(instanceInterfaces(ifs))
	interfacesListExtracted, err := extractInstanceInterfaceToListRead(ifs)
	if err != nil {
		return diag.FromErr(err)
	}

	var interfacesList []interface{}
	for order, iFace := range interfacesListAPI {
		if len(iFace.IPAssignments) == 0 {
			continue
		}

		portID := iFace.PortID
		for _, assignment := range iFace.IPAssignments {
			subnetID := assignment.SubnetID
			ipAddress := assignment.IPAddress.String()

			var interfaceOpts instances.InterfaceOpts
			for _, interfaceExtracted := range interfacesListExtracted {
				if interfaceExtracted.SubnetID == subnetID ||
					interfaceExtracted.IPAddress == ipAddress ||
					interfaceExtracted.PortID == portID {
					interfaceOpts = interfaceExtracted
					break
				}
			}

			i := make(map[string]interface{})
			i["type"] = interfaceOpts.Type.String()
			i["order"] = order
			i["network_id"] = iFace.NetworkID
			i["subnet_id"] = subnetID
			i["port_id"] = portID
			i["is_parent"] = true
			if interfaceOpts.FloatingIP != nil {
				i["fip_source"] = interfaceOpts.FloatingIP.Source.String()
				i["existing_fip_id"] = interfaceOpts.FloatingIP.ExistingFloatingID
			}
			i["ip_address"] = ipAddress

			interfacesList = append(interfacesList, i)
		}

		for _, iFaceSubPort := range iFace.SubPorts {
			subPortID := iFaceSubPort.PortID
			for _, assignmentSubPort := range iFaceSubPort.IPAssignments {
				assignmentSubnetID := assignmentSubPort.SubnetID
				assignmentIPAddress := assignmentSubPort.IPAddress.String()

				var subPortInterfaceOpts instances.InterfaceOpts
				for _, interfaceExtracted := range interfacesListExtracted {
					if interfaceExtracted.SubnetID == assignmentSubnetID ||
						interfaceExtracted.IPAddress == assignmentIPAddress ||
						interfaceExtracted.PortID == subPortID {
						subPortInterfaceOpts = interfaceExtracted
						break
					}
				}

				i := make(map[string]interface{})

				i["type"] = subPortInterfaceOpts.Type.String()
				i["order"] = order
				i["network_id"] = iFaceSubPort.NetworkID
				i["subnet_id"] = assignmentSubnetID
				i["port_id"] = subPortID
				i["is_parent"] = false
				if subPortInterfaceOpts.FloatingIP != nil {
					i["fip_source"] = subPortInterfaceOpts.FloatingIP.Source.String()
					i["existing_fip_id"] = subPortInterfaceOpts.FloatingIP.ExistingFloatingID
				}
				i["ip_address"] = assignmentIPAddress

				interfacesList = append(interfacesList, i)
			}
		}
	}
	if err := d.Set("interface", interfacesList); err != nil {
		return diag.FromErr(err)
	}

	if metadataRaw, ok := d.GetOk("metadata"); ok {
		metadata := metadataRaw.([]interface{})
		sliced := make([]map[string]string, len(metadata))
		for i, data := range metadata {
			d := data.(map[string]interface{})
			mdata := make(map[string]string, 2)
			md, err := instances.MetadataGet(client, instanceID, d["key"].(string)).Extract()
			if err != nil {
				return diag.Errorf("cannot get metadata with key: %s. Error: %s", instanceID, err)
			}
			mdata["key"] = md.Key
			mdata["value"] = md.Value
			sliced[i] = mdata
		}
		d.Set("metadata", sliced)
	} else {
		metadata := d.Get("metadata_map").(map[string]interface{})
		newMetadata := make(map[string]interface{}, len(metadata))
		for k := range metadata {
			md, err := instances.MetadataGet(client, instanceID, k).Extract()
			if err != nil {
				return diag.Errorf("cannot get metadata with key: %s. Error: %s", instanceID, err)
			}
			newMetadata[k] = md.Value
		}
		if err := d.Set("metadata_map", newMetadata); err != nil {
			return diag.FromErr(err)
		}
	}

	addresses := []map[string][]map[string]string{}
	for _, data := range instance.Addresses {
		d := map[string][]map[string]string{}
		netd := make([]map[string]string, len(data))
		for i, iaddr := range data {
			ndata := make(map[string]string, 2)
			ndata["type"] = iaddr.Type.String()
			ndata["addr"] = iaddr.Address.String()
			netd[i] = ndata
		}
		d["net"] = netd
		addresses = append(addresses, d)
	}
	if err := d.Set("addresses", addresses); err != nil {
		return diag.FromErr(err)
	}

	fields := []string{"user_data", "app_config"}
	revertState(d, &fields)

	log.Println("[DEBUG] Finish Instance reading")

	return diags
}

func resourceBmInstanceUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	log.Println("[DEBUG] Start Baremetal Instance updating")
	instanceID := d.Id()
	log.Printf("[DEBUG] Instance id = %s", instanceID)
	config := m.(*Config)
	provider := config.Provider
	client, err := CreateClient(provider, d, InstancePoint, VersionPointV1)
	if err != nil {
		return diag.FromErr(err)
	}

	if d.HasChange("name") {
		nameTemplates := d.Get("name_templates").([]interface{})
		nameTemplate := d.Get("name_template").(string)
		if len(nameTemplate) == 0 && len(nameTemplates) == 0 {
			opts := instances.RenameInstanceOpts{
				Name: d.Get("name").(string),
			}
			if _, err := instances.RenameInstance(client, instanceID, opts).Extract(); err != nil {
				return diag.FromErr(err)
			}
		}
	}

	if d.HasChange("metadata") {
		omd, nmd := d.GetChange("metadata")
		if len(omd.([]interface{})) > 0 {
			for _, data := range omd.([]interface{}) {
				d := data.(map[string]interface{})
				k := d["key"].(string)
				err := instances.MetadataDelete(client, instanceID, k).Err
				if err != nil {
					return diag.Errorf("cannot delete metadata key: %s. Error: %s", k, err)
				}
			}
		}
		if len(nmd.([]interface{})) > 0 {
			var MetaData []instances.MetadataOpts
			for _, data := range nmd.([]interface{}) {
				d := data.(map[string]interface{})
				var md instances.MetadataOpts
				md.Key = d["key"].(string)
				md.Value = d["value"].(string)
				MetaData = append(MetaData, md)
			}
			createOpts := instances.MetadataSetOpts{
				Metadata: MetaData,
			}
			err := instances.MetadataCreate(client, instanceID, createOpts).Err
			if err != nil {
				return diag.Errorf("cannot create metadata. Error: %s", err)
			}
		}
	} else if d.HasChange("metadata_map") {
		omd, nmd := d.GetChange("metadata_map")
		if len(omd.(map[string]interface{})) > 0 {
			for k := range omd.(map[string]interface{}) {
				err := instances.MetadataDelete(client, instanceID, k).Err
				if err != nil {
					return diag.Errorf("cannot delete metadata key: %s. Error: %s", k, err)
				}
			}
		}
		if len(nmd.(map[string]interface{})) > 0 {
			var MetaData []instances.MetadataOpts
			for k, v := range nmd.(map[string]interface{}) {
				md := instances.MetadataOpts{
					Key:   k,
					Value: v.(string),
				}
				MetaData = append(MetaData, md)
			}
			createOpts := instances.MetadataSetOpts{
				Metadata: MetaData,
			}
			err := instances.MetadataCreate(client, instanceID, createOpts).Err
			if err != nil {
				return diag.Errorf("cannot create metadata. Error: %s", err)
			}
		}
	}

	if d.HasChange("interface") {
		ifsOldRaw, ifsNewRaw := d.GetChange("interface")

		ifsOld := ifsOldRaw.([]interface{})
		ifsNew := ifsNewRaw.([]interface{})

		for _, i := range ifsOld {
			iface := i.(map[string]interface{})
			if isInterfaceContains(iface, ifsNew) {
				log.Println("[DEBUG] Skipped, dont need detach")
				continue
			}

			if iface["is_parent"].(bool) {
				return diag.Errorf("could not detach trunk interface")
			}

			var opts instances.InterfaceOpts
			opts.PortID = iface["port_id"].(string)
			opts.IPAddress = iface["ip_address"].(string)

			log.Printf("[DEBUG] detach interface: %+v", opts)
			results, err := instances.DetachInterface(client, instanceID, opts).Extract()
			if err != nil {
				return diag.FromErr(err)
			}
			taskID := results.Tasks[0]
			_, err = tasks.WaitTaskAndReturnResult(client, taskID, true, InstanceCreatingTimeout, func(task tasks.TaskID) (interface{}, error) {
				taskInfo, err := tasks.Get(client, string(task)).Extract()
				if err != nil {
					return nil, fmt.Errorf("cannot get task with ID: %s. Error: %w, task: %+v", task, err, taskInfo)
				}
				return nil, nil
			},
			)
			if err != nil {
				return diag.FromErr(err)
			}
		}

		currentIfs, err := instances.ListInterfacesAll(client, d.Id())
		if err != nil {
			return diag.FromErr(err)
		}

		sort.Sort(instanceInterfaces(ifsNew))
		for _, i := range ifsNew {
			iface := i.(map[string]interface{})
			if isInterfaceContains(iface, ifsOld) {
				log.Println("[DEBUG] Skipped, dont need attach")
				continue
			}
			if isInterfaceAttached(currentIfs, iface) {
				continue
			}

			iType := types.InterfaceType(iface["type"].(string))
			opts := instances.InterfaceOpts{Type: iType}
			switch iType {
			case types.SubnetInterfaceType:
				opts.SubnetID = iface["subnet_id"].(string)
			case types.AnySubnetInterfaceType:
				opts.NetworkID = iface["network_id"].(string)
			case types.ReservedFixedIPType:
				opts.PortID = iface["port_id"].(string)
			case types.ExternalInterfaceType:
			}

			log.Printf("[DEBUG] attach interface: %+v", opts)
			results, err := instances.AttachInterface(client, instanceID, opts).Extract()
			if err != nil {
				return diag.Errorf("cannot attach interface: %s. Error: %s", iType, err)
			}
			taskID := results.Tasks[0]
			_, err = tasks.WaitTaskAndReturnResult(client, taskID, true, InstanceCreatingTimeout, func(task tasks.TaskID) (interface{}, error) {
				taskInfo, err := tasks.Get(client, string(task)).Extract()
				if err != nil {
					return nil, fmt.Errorf("cannot get task with ID: %s. Error: %w, task: %+v", task, err, taskInfo)
				}
				return nil, nil
			},
			)
			if err != nil {
				return diag.FromErr(err)
			}
		}
	}

	d.Set("last_updated", time.Now().Format(time.RFC850))
	log.Println("[DEBUG] Finish Instance updating")

	return resourceBmInstanceRead(ctx, d, m)
}

func resourceBmInstanceDelete(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	log.Println("[DEBUG] Start Baremetal Instance deleting")
	var diags diag.Diagnostics
	config := m.(*Config)
	provider := config.Provider
	instanceID := d.Id()
	log.Printf("[DEBUG] Instance id = %s", instanceID)

	client, err := CreateClient(provider, d, InstancePoint, VersionPointV1)
	if err != nil {
		return diag.FromErr(err)
	}

	var delOpts instances.DeleteOpts
	delOpts.DeleteFloatings = true

	results, err := instances.Delete(client, instanceID, delOpts).Extract()
	if err != nil {
		return diag.FromErr(err)
	}
	taskID := results.Tasks[0]
	log.Printf("[DEBUG] Task id (%s)", taskID)
	_, err = tasks.WaitTaskAndReturnResult(client, taskID, true, BmInstanceDeleting, func(task tasks.TaskID) (interface{}, error) {
		_, err := instances.Get(client, instanceID).Extract()
		if err == nil {
			return nil, fmt.Errorf("cannot delete instance with ID: %s", instanceID)
		}
		var errDefault404 edgecloud.Default404Error
		if errors.As(err, &errDefault404) {
			return nil, nil
		}
		return nil, fmt.Errorf("extracting Instance resource error: %w", err)
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	log.Printf("[DEBUG] Finish of Instance deleting")

	return diags
}
