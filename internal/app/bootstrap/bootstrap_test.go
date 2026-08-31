package bootstrap_test

import (
	"testing/fstest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/config/pkg/registry"
	"github.com/ensoria/ensoria-template/internal/app/bootstrap"
	"github.com/ensoria/ensoria-template/internal/plamo/restkit"
)

const (
	basePath  = "internal"
	configDir = "config"

	specYAML = "var_locations: [dotenv]\n"

	// The settings every configuration needs, minus the one each spec varies.
	baseDotEnv = "HTTP_PORT=8080\n" +
		"DB_DRIVER=postgres\n" +
		"DB_HOST=localhost\n" +
		"DB_PORT=5432\n" +
		"DB_USER=postgres\n" +
		"DB_PASSWORD=secret\n" +
		"DB_NAME=app\n" +
		"REDIS_HOST=localhost\n" +
		"REDIS_PORT=6379\n"
)

// loadedRegistry builds a registry of this spec's own, holding a default module
// whose only interesting value is the log level. Taking the registry as an
// argument is what lets these specs do that: nothing here touches the
// process-wide one the applications read.
func loadedRegistry(envName, logLevel string) *registry.Registry {
	GinkgoHelper()

	fsys := fstest.MapFS{
		"internal/config/" + envName + ".yml": {Data: []byte(specYAML)},
		"internal/config/.env":                {Data: []byte(baseDotEnv + "LOG_LEVEL=" + logLevel + "\n")},
	}

	reg := registry.New()
	Expect(reg.InitializeConfiguration(envName, fsys, basePath, configDir)).To(Succeed())
	return reg
}

// Every application applies the same global settings before it builds its
// graph. They are applied from one place so that adding an application, or a
// setting, cannot leave one of them behind — which is exactly how the strict
// declaration mode came to be missing from the scheduler.
var _ = Describe("ApplyGlobalSettings", func() {
	// Both settings this applies are process-wide, so each spec puts the strict
	// flag back where it found it.
	var strictBefore bool

	BeforeEach(func() { strictBefore = restkit.StrictDeclarations() })
	AfterEach(func() { restkit.SetStrictDeclarations(strictBefore) })

	Describe("the strict declaration mode", func() {
		It("turns it on in the environments a developer works in", func() {
			for _, envName := range []string{"local", "test", "development"} {
				restkit.SetStrictDeclarations(false)

				_, err := bootstrap.ApplyGlobalSettings(&envName, loadedRegistry(envName, "info"))

				Expect(err).NotTo(HaveOccurred())
				Expect(restkit.StrictDeclarations()).To(BeTrue(), "env %s", envName)
			}
		})

		It("leaves it off anywhere a real request could hit it", func() {
			for _, envName := range []string{"staging", "production"} {
				restkit.SetStrictDeclarations(true)

				_, err := bootstrap.ApplyGlobalSettings(&envName, loadedRegistry(envName, "info"))

				Expect(err).NotTo(HaveOccurred())
				Expect(restkit.StrictDeclarations()).To(BeFalse(), "env %s", envName)
			}
		})
	})

	Describe("fx's own construction log", func() {
		It("is asked for only at the debug level", func() {
			envName := "local"

			outputFxLog, err := bootstrap.ApplyGlobalSettings(&envName, loadedRegistry(envName, "debug"))

			Expect(err).NotTo(HaveOccurred())
			Expect(outputFxLog).To(BeTrue())
		})

		It("is left off at every other level", func() {
			envName := "local"

			for _, level := range []string{"info", "warn", "error"} {
				outputFxLog, err := bootstrap.ApplyGlobalSettings(&envName, loadedRegistry(envName, level))

				Expect(err).NotTo(HaveOccurred())
				Expect(outputFxLog).To(BeFalse(), "level %s", level)
			}
		})
	})

	Describe("when the configuration cannot be read", func() {
		It("reports the failure rather than starting with unapplied settings", func() {
			envName := "local"
			reg := registry.New()
			Expect(reg.LoadConfigurationFiles(fstest.MapFS{}, envName, map[string]string{}, true)).To(Succeed())

			outputFxLog, err := bootstrap.ApplyGlobalSettings(&envName, reg)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("app initialization error"))
			Expect(outputFxLog).To(BeFalse())
		})
	})
})
