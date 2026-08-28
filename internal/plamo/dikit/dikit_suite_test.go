package dikit_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDikit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dikit Suite")
}
