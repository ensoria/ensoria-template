package initiator_test

import (
	"bytes"
	"encoding/json"

	"github.com/ensoria/loggear/pkg/loggear"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// captureLogRecords redirects the global logger into a buffer for the duration
// of write, and returns the records it wrote, decoded.
//
// The logger is global, so the previous one is put back afterwards.
func captureLogRecords(write func()) []map[string]any {
	GinkgoHelper()

	var buf bytes.Buffer
	previous := loggear.GetLogger()
	loggear.SetLogger(loggear.NewSlogLogger(loggear.WithOutput(&buf)))
	defer loggear.SetLogger(previous)

	write()

	var records []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var record map[string]any
		Expect(json.Unmarshal(line, &record)).To(Succeed())
		records = append(records, record)
	}
	return records
}

// recordsOfType keeps the records carrying the given type.
func recordsOfType(records []map[string]any, logType string) []map[string]any {
	var out []map[string]any
	for _, r := range records {
		if r["type"] == logType {
			out = append(out, r)
		}
	}
	return out
}
