package acceptance

import (
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/sovereignhybridcompute/terraform-provider-shc/provider"
)

const TestNamePrefix = "tf-acc-test-"

func PreCheck(t *testing.T) {
	if os.Getenv("SHC_API_KEY") == "" {
		t.Fatal("SHC_API_KEY must be set for acceptance tests")
	}
}

func OptInTest(t *testing.T) {
	t.Helper()
	if os.Getenv("ACC_OPT_IN_TESTS") == "" {
		t.Skip("Skipping opt-in test. Set ACC_OPT_IN_TESTS=1 to run.")
	}
}

func LongRunningTest(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_LONG_TESTS") == "" {
		t.Skip("Skipping long-running test. Set RUN_LONG_TESTS=1 to run.")
	}
}

func RandHostname(prefix string) string {
	return TestNamePrefix + prefix + "-" + acctest.RandString(8)
}

func RandHostnameShort() string {
	return TestNamePrefix + "vm-" + acctest.RandString(8)
}

func TestClient() *provider.SHCClient {
	return provider.NewSHCClient(os.Getenv("SHC_API_KEY"), "")
}

func ProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"shc": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func TestMain(m *testing.M) {
	_ = rand.New(rand.NewSource(time.Now().UnixNano()))
	_ = acctest.RandString(1)
	resource.TestMain(m)
}
