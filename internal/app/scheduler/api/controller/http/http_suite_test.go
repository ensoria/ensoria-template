package http_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSchedulerAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "app/scheduler/api/controller/http Suite")
}
