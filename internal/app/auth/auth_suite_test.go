// The tests live in package auth rather than auth_test because they exercise
// the guard on the secret shipped with the template, which is not exported.
package auth

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAuth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "app/auth Suite")
}
