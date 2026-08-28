package mb_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInfraMB(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Infra MB Suite")
}
