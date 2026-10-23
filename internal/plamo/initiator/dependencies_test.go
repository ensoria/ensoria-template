package initiator_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// authkitImportPath is the package that holds the verified caller.
const authkitImportPath = "github.com/ensoria/ensoria-template/internal/plamo/authkit"

// templateRoot is the repository root, seen from this package's directory.
const templateRoot = "../../.."

// internalPathPackages are the directories of the paths work travels along
// inside the system, where nothing is verified: the message broker (wiring,
// declarations, connections, every module's subscriptions and publications),
// the worker (wiring and every module's jobs) and the scheduler (wiring and
// every module's tasks; its management endpoints are an HTTP entry point and
// are not on the list).
func internalPathPackages() []string {
	GinkgoHelper()

	dirs := []string{
		"internal/app/mb", "internal/plamo/mbkit", "internal/infra/mb",
		"internal/app/worker", "internal/app/worker/job",
		"internal/app/scheduler", "internal/app/scheduler/task",
	}
	for _, pattern := range []string{"internal/module/*/controller/mb", "internal/module/*/job", "internal/module/*/task"} {
		matches, err := filepath.Glob(filepath.Join(templateRoot, pattern))
		Expect(err).NotTo(HaveOccurred())
		for _, m := range matches {
			rel, err := filepath.Rel(templateRoot, m)
			Expect(err).NotTo(HaveOccurred())
			dirs = append(dirs, rel)
		}
	}
	return dirs
}

// importsOf lists the packages the non-test Go files in dir import.
func importsOf(dir string) map[string][]string {
	GinkgoHelper()

	entries, err := os.ReadDir(filepath.Join(templateRoot, dir))
	Expect(err).NotTo(HaveOccurred())
	byFile := map[string][]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(templateRoot, dir, name), nil, parser.ImportsOnly)
		Expect(err).NotTo(HaveOccurred())
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			Expect(err).NotTo(HaveOccurred())
			byFile[filepath.Join(dir, name)] = append(byFile[filepath.Join(dir, name)], path)
		}
	}
	return byFile
}

// A message, a job or a scheduled task carries who started the work as an
// Initiator, never as a caller: nothing on these paths was verified, so a
// Principal there would be one that authorization trusts without grounds. The
// dependency makes that visible: these paths read this package and never import
// authkit, and a change that starts to is caught here rather than in review.
var _ = Describe("the internal paths", func() {
	It("finds the packages it checks", func() {
		Expect(internalPathPackages()).To(ContainElements(
			"internal/module/user/controller/mb", "internal/module/user/job", "internal/module/user/task"))
	})

	It("do not import authkit", func() {
		for _, dir := range internalPathPackages() {
			for file, imports := range importsOf(dir) {
				Expect(imports).NotTo(ContainElement(authkitImportPath), "%s imports authkit", file)
			}
		}
	})
})
