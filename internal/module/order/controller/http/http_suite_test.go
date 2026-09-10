package http_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOrderHTTP(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "module/order/controller/http Suite")
}
