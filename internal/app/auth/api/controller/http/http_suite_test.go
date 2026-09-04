package http_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSessionEndpoints(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Session Endpoints Suite")
}
