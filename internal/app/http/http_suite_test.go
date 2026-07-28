package http

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHTTPApp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTP App Suite")
}
