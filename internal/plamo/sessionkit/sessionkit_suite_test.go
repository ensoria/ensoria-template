package sessionkit_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSessionKit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SessionKit Suite")
}
