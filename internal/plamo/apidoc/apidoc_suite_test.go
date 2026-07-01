package apidoc_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApidoc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Apidoc Suite")
}
