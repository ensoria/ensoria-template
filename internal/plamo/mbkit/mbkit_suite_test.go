package mbkit_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMbkit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mbkit Suite")
}
