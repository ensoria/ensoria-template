package grpc

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/loggear/pkg/loggear"
)

var _ = Describe("LogConfig", func() {
	// The two settings a deployment changes come from the configuration.
	It("takes the sampling and the truncation length from the configuration", func() {
		cfg := LogConfig(&appconfig.GRPC{
			LogSuccessSampleRate: 0.05,
			LogMaxHeaderValueLen: 128,
		})

		Expect(cfg.SuccessSampleRate).To(Equal(0.05))
		Expect(cfg.MaxHeaderValueLen).To(Equal(128))
	})

	// The application's own policy stays in code: it is reviewed next to the
	// services it describes and does not change because a deployment moved.
	It("keeps the header policy in the template rather than in the configuration", func() {
		cfg := LogConfig(&appconfig.GRPC{})

		Expect(cfg.SensitiveHeaders).To(ContainElements("authorization", "x-api-key", "cookie"))
		Expect(cfg.IncludeHeaders).To(ContainElement("user-agent"))
		Expect(cfg.MaskWith).To(Equal("***"))
		Expect(cfg.SkipMethodPrefixes).To(ContainElement("/grpc.health.v1."))
	})
})

var _ = Describe("NewGRPCServer", func() {
	// A sample rate of 0 means "log no successful call", but the interceptor
	// reads a zero as "not configured" and would log every one of them, so the
	// setting is honored by silencing the success loggers instead. What is
	// checked here is that the server is still built either way — the swap
	// itself is inside NewGRPCServer, where nothing else can observe it.
	It("builds a server whether or not successful calls are logged", func() {
		logger := loggear.GetLogger()

		Expect(NewGRPCServer(logger, &appconfig.GRPC{LogSuccessSampleRate: 0})).NotTo(BeNil())
		Expect(NewGRPCServer(logger, &appconfig.GRPC{LogSuccessSampleRate: 0.3})).NotTo(BeNil())
	})
})
