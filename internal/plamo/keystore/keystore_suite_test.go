package keystore_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKeyStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KeyStore Suite")
}
