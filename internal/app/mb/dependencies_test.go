package mb_test

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

// mbSidePackages are the directories on the message broker path: the wiring,
// the declarations, the connections, and every module's subscriptions and
// publications.
func mbSidePackages() []string {
	GinkgoHelper()

	dirs := []string{"internal/app/mb", "internal/plamo/mbkit", "internal/infra/mb"}
	modules, err := filepath.Glob(filepath.Join(templateRoot, "internal/module/*/controller/mb"))
	Expect(err).NotTo(HaveOccurred())
	for _, m := range modules {
		rel, err := filepath.Rel(templateRoot, m)
		Expect(err).NotTo(HaveOccurred())
		dirs = append(dirs, rel)
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

// A message carries who started the work as an Initiator, never as a caller:
// nothing on this path was verified, so a Principal here would be one that
// authorization trusts without grounds. The dependency
// makes that visible: the broker path reads plamo/initiator and never imports
// authkit, and a change that starts to is caught here rather than in review.
var _ = Describe("the message broker path", func() {
	It("finds the packages it checks", func() {
		Expect(mbSidePackages()).To(ContainElement("internal/module/user/controller/mb"))
	})

	It("does not import authkit", func() {
		for _, dir := range mbSidePackages() {
			for file, imports := range importsOf(dir) {
				Expect(imports).NotTo(ContainElement(authkitImportPath), "%s imports authkit", file)
			}
		}
	})
})
