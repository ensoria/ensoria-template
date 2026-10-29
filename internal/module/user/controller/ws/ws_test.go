package ws_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/ensoria-template/internal/module/user/dto"
	"github.com/ensoria/loggear/pkg/loggear"
)

// lockedBuffer is a log destination the server's goroutines and the spec can
// share without racing.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLog runs write with every record going to a buffer, and returns the
// raw text along with the records named msg.
func captureLog(msg string, write func()) (string, []map[string]any) {
	GinkgoHelper()

	var out lockedBuffer
	previous := loggear.GetLogger()
	loggear.SetLogger(loggear.NewSlogLogger(loggear.WithOutput(&out)))
	write()
	loggear.SetLogger(previous)

	raw := out.String()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		var record map[string]any
		if json.Unmarshal([]byte(line), &record) == nil && record["msg"] == msg {
			records = append(records, record)
		}
	}
	return raw, records
}

const receivedMsg = "WebSocket message received"

var _ = Describe("LogOnMessage", func() {
	// Payloads carry whatever a client typed — names, addresses, a token
	// pasted by mistake — and arrive before anything has been validated or
	// authorized. The record says which message arrived, never what it said,
	// as the HTTP access record names a request without its body.
	It("records which message arrived and how big it was, never what it carried", func() {
		server := serveUserChannel()
		frame := `{"type":"user.echo","data":{"message":"my address is 1-2-3 Sakura"}}`

		raw, records := captureLog(receivedMsg, func() {
			conn := connect(server, "")
			send(conn, frame)
			var reply dto.UserEchoReply
			next(conn, &reply)
		})

		Expect(records).To(HaveLen(1))
		Expect(records[0]).To(HaveKeyWithValue("message_type", "user.echo"))
		Expect(records[0]).To(HaveKeyWithValue("bytes", BeNumerically("==", len(frame))))
		Expect(records[0]).To(HaveKey("remote_addr"))
		Expect(raw).NotTo(ContainSubstring("Sakura"))
	})

	It("records a frame that is not an envelope without its contents", func() {
		server := serveUserChannel()

		raw, records := captureLog(receivedMsg, func() {
			conn := connect(server, "")
			send(conn, `my password is hunter2`)
			var answer map[string]any
			next(conn, &answer)
		})

		Expect(records).To(HaveLen(1))
		Expect(records[0]).To(HaveKeyWithValue("message_type", ""))
		Expect(raw).NotTo(ContainSubstring("hunter2"))
	})

	// The type is whatever the client wrote, and a public channel lets anyone
	// write. Bounding it keeps the record from becoming a way to put arbitrary
	// text into the logs after all.
	It("bounds a message type a client made long", func() {
		server := serveUserChannel()
		long := strings.Repeat("あ", 500)

		_, records := captureLog(receivedMsg, func() {
			conn := connect(server, "")
			send(conn, `{"type":"`+long+`"}`)
			var answer map[string]any
			next(conn, &answer)
		})

		Expect(records).To(HaveLen(1))
		logged, _ := records[0]["message_type"].(string)
		Expect(logged).To(HavePrefix("あ"))
		Expect(utf8.RuneCountInString(logged)).To(BeNumerically("<", 100))
		Expect(utf8.ValidString(logged)).To(BeTrue())
	})
})
