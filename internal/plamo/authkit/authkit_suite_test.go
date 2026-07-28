package authkit_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAuthkit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Authkit Suite")
}
