package operatorkclient_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOperatorKClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OperatorKClient Suite")
}
