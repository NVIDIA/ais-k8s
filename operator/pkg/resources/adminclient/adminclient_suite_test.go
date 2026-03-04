package adminclient

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAdminClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AdminClient Suite")
}
