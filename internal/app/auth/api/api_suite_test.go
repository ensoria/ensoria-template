package api_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSessionAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Session API Suite")
}
