package worker

import (
	"bytes"
	"encoding/json"
	"sync"

	"github.com/ensoria/loggear/pkg/loggear"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// lockedBuffer is a buffer safe to write from the worker's goroutines while the
// spec reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.buf.Bytes())
}

// captureLogRecords redirects the global logger into a buffer for the duration
// of write, and returns the records it wrote, decoded.
func captureLogRecords(write func()) []map[string]any {
	GinkgoHelper()

	var buf lockedBuffer
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
