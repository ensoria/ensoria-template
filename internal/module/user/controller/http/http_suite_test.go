package http_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUserHTTP(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "module/user/controller/http Suite")
}
