package provider

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/nightshift-habits/terraform-provider-qualtive/internal/client"
)

const (
	envAccContainer   = "QUALTIVE_ACC_CONTAINER"
	envAccWorkspaceID = "QUALTIVE_ACC_WORKSPACE_ID"
)

var testProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"qualtive": providerserver.NewProtocol6WithError(New("test")()),
}

// testEnv is where a test's enquiries live: the in-memory fake by default,
// or a real Admin API in TestAcc* tests.
type testEnv struct {
	container     string
	workspaceID   int64
	providerBlock string
	client        *client.Client
	fake          *fakeAdminAPI
	run           func(t *testing.T, c resource.TestCase)
}

func newFakeEnv(t *testing.T) *testEnv {
	fake := newFakeAdminAPI(t)
	return &testEnv{
		container:   "acme",
		workspaceID: 17,
		providerBlock: fmt.Sprintf(`
provider "qualtive" {
  endpoint = %q
  api_key  = %q
}
`, fake.server.URL, fakeAPIKey),
		client: client.New(fake.server.URL, fakeAPIKey, "test"),
		fake:   fake,
		run:    func(t *testing.T, c resource.TestCase) { resource.UnitTest(t, c) },
	}
}

// newAccEnv targets a real Admin API, typically a local backend. It needs
// TF_ACC=1, QUALTIVE_API_KEY, QUALTIVE_ACC_CONTAINER and QUALTIVE_ACC_WORKSPACE_ID,
// and optionally QUALTIVE_ENDPOINT.
func newAccEnv(t *testing.T) *testEnv {
	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skipf("set %s=1 to run acceptance tests against a real Admin API", resource.EnvTfAcc)
	}
	for _, name := range []string{EnvAPIKey, envAccContainer, envAccWorkspaceID} {
		if os.Getenv(name) == "" {
			t.Fatalf("%s must be set for acceptance tests", name)
		}
	}
	workspaceID, err := strconv.ParseInt(os.Getenv(envAccWorkspaceID), 10, 64)
	if err != nil {
		t.Fatalf("%s must be a numeric workspace ID: %v", envAccWorkspaceID, err)
	}
	endpoint := firstNonEmpty(os.Getenv(EnvEndpoint), DefaultEndpoint)
	return &testEnv{
		container:     os.Getenv(envAccContainer),
		workspaceID:   workspaceID,
		providerBlock: `provider "qualtive" {}`,
		client:        client.New(endpoint, os.Getenv(EnvAPIKey), "terraform-provider-qualtive-acc"),
		run:           func(t *testing.T, c resource.TestCase) { resource.Test(t, c) },
	}
}
