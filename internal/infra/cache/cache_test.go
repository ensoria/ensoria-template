package cache

import (
	"crypto/tls"

	"github.com/ensoria/config/pkg/appconfig"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func boolPtr(v bool) *bool { return &v }

var _ = Describe("redisOptions", func() {
	It("joins the host and port into the address go-redis dials", func() {
		opts := redisOptions(&appconfig.Redis{Host: "redis.internal", Port: 6379})

		Expect(opts.Addr).To(Equal("redis.internal:6379"))
	})

	// net.JoinHostPort brackets an IPv6 literal; fmt.Sprintf("%s:%d") would
	// produce an address the dialer cannot parse.
	It("brackets an IPv6 host", func() {
		opts := redisOptions(&appconfig.Redis{Host: "::1", Port: 6379})

		Expect(opts.Addr).To(Equal("[::1]:6379"))
	})

	It("carries the credentials and the database number", func() {
		opts := redisOptions(&appconfig.Redis{
			Host:     "redis.internal",
			Port:     6379,
			User:     "app",
			Password: "secret",
			DB:       3,
		})

		Expect(opts.Username).To(Equal("app"))
		Expect(opts.Password).To(Equal("secret"))
		Expect(opts.DB).To(Equal(3))
	})

	// Database 0 is a real database, not "unset", so it has to survive the
	// conversion: it is where the job queue lives.
	It("keeps database 0", func() {
		opts := redisOptions(&appconfig.Redis{Host: "redis.internal", Port: 6379, DB: 0})

		Expect(opts.DB).To(BeZero())
	})

	Describe("TLS", func() {
		// A non-nil TLSConfig is what turns encryption on, so an unset or
		// disabled setting must leave it nil rather than pass an empty config.
		It("leaves the TLS config nil when TLS is unset", func() {
			opts := redisOptions(&appconfig.Redis{Host: "redis.internal", Port: 6379})

			Expect(opts.TLSConfig).To(BeNil())
		})

		It("leaves the TLS config nil when TLS is explicitly off", func() {
			opts := redisOptions(&appconfig.Redis{Host: "redis.internal", Port: 6379, TLS: boolPtr(false)})

			Expect(opts.TLSConfig).To(BeNil())
		})

		It("sets a TLS config with a floor of TLS 1.2 when TLS is on", func() {
			opts := redisOptions(&appconfig.Redis{Host: "redis.internal", Port: 6379, TLS: boolPtr(true)})

			Expect(opts.TLSConfig).NotTo(BeNil())
			Expect(opts.TLSConfig.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
		})

		// ServerName is filled in by tls.DialWithDialer from the address being
		// dialed, so setting it here would only risk disagreeing with it.
		It("leaves the server name to the dialer", func() {
			opts := redisOptions(&appconfig.Redis{Host: "redis.internal", Port: 6379, TLS: boolPtr(true)})

			Expect(opts.TLSConfig.ServerName).To(BeEmpty())
			Expect(opts.TLSConfig.InsecureSkipVerify).To(BeFalse())
		})
	})

	It("returns nil for a nil configuration", func() {
		Expect(redisOptions(nil)).To(BeNil())
	})
})
