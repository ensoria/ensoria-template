package mb_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMB(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "App MB Suite")
}
