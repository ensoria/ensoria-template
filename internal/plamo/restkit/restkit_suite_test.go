package restkit_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRestkit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Restkit Suite")
}
