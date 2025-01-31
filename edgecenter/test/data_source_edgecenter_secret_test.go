//go:build cloud_data_source

package edgecenter_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"

	"github.com/Edge-Center/edgecentercloud-go/edgecenter/secret/v1/secrets"
	secretsV2 "github.com/Edge-Center/edgecentercloud-go/edgecenter/secret/v2/secrets"
	"github.com/Edge-Center/edgecentercloud-go/edgecenter/task/v1/tasks"
	"github.com/Edge-Center/terraform-provider-edgecenter/edgecenter"
)

func TestAccSecretDataSource(t *testing.T) {
	t.Parallel()
	cfg, err := createTestConfig()
	if err != nil {
		t.Fatal(err)
	}

	client, err := createTestClient(cfg.Provider, edgecenter.SecretPoint, edgecenter.VersionPointV1)
	if err != nil {
		t.Fatal(err)
	}
	clientV2, err := createTestClient(cfg.Provider, edgecenter.SecretPoint, edgecenter.VersionPointV2)
	if err != nil {
		t.Fatal(err)
	}

	opts := secretsV2.CreateOpts{
		Name: secretTestName,
		Payload: secretsV2.PayloadOpts{
			CertificateChain: certificateChain,
			Certificate:      certificate,
			PrivateKey:       privateKey,
		},
	}
	results, err := secretsV2.Create(clientV2, opts).Extract()
	if err != nil {
		t.Fatal(err)
	}

	taskID := results.Tasks[0]
	secretID, err := tasks.WaitTaskAndReturnResult(client, taskID, true, edgecenter.SecretCreatingTimeout, func(task tasks.TaskID) (interface{}, error) {
		taskInfo, err := tasks.Get(client, string(task)).Extract()
		if err != nil {
			return nil, fmt.Errorf("cannot get task with ID: %s. Error: %w", task, err)
		}
		Secret, err := secrets.ExtractSecretIDFromTask(taskInfo)
		if err != nil {
			return nil, fmt.Errorf("cannot retrieve Secret ID from task info: %w", err)
		}
		return Secret, nil
	},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer secrets.Delete(client, secretID.(string))

	resourceName := "data.edgecenter_secret.acctest"
	kpTemplate := fmt.Sprintf(`
	data "edgecenter_secret" "acctest" {
	  %s
      %s
      name = "%s"
	}
	`, projectInfo(), regionInfo(), secretTestName)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: kpTemplate,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckResourceExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", secretTestName),
				),
			},
		},
	})
}
