package http_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWorkerAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "app/worker/api/controller/http Suite")
}
