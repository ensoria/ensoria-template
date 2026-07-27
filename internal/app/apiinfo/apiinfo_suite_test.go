package apiinfo_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApiinfo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Apiinfo Suite")
}
