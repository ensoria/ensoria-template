package msgdoc_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMsgdoc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Msgdoc Suite")
}
