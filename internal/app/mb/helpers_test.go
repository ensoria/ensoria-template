package mb_test

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/ensoria/loggear/pkg/loggear"
	"github.com/ensoria/mb/pkg/mb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// captureLogRecords redirects the global logger into a buffer for the duration
// of write, and returns the records it wrote, decoded.
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

// published is one message a fakePublisher was asked to send.
type published struct {
	target   string
	metadata map[string]string
}

// fakePublisher records what it is asked to publish instead of reaching a
// broker. carries is what it answers to CarriesMetadata.
type fakePublisher struct {
	carries bool
	sent    []published
}

func (p *fakePublisher) Connect(context.Context) error               { return nil }
func (p *fakePublisher) SetOptions(...mb.PublishOption) mb.Publisher { return p }
func (p *fakePublisher) CarriesMetadata() bool                       { return p.carries }
func (p *fakePublisher) Ping(context.Context) error                  { return nil }
func (p *fakePublisher) Close() error                                { return nil }
func (p *fakePublisher) Publish(_ context.Context, target string, _ []byte, metadata map[string]string, _ ...mb.PublishOption) error {
	p.sent = append(p.sent, published{target: target, metadata: metadata})
	return nil
}

// fakeSubscriber keeps the handler it is given, so a spec can deliver a message
// the way a broker would.
type fakeSubscriber struct {
	handlers map[string]mb.MessageHandler
}

func newFakeSubscriber() *fakeSubscriber {
	return &fakeSubscriber{handlers: map[string]mb.MessageHandler{}}
}

func (s *fakeSubscriber) Connect(context.Context) error                  { return nil }
func (s *fakeSubscriber) SetOptions(...mb.SubscribeOption) mb.Subscriber { return s }
func (s *fakeSubscriber) Ping(context.Context) error                     { return nil }
func (s *fakeSubscriber) Close() error                                   { return nil }
func (s *fakeSubscriber) Subscribe(_ context.Context, target string, handler mb.MessageHandler, _ ...mb.SubscribeOption) error {
	s.handlers[target] = handler
	return nil
}

// deliver hands a message to the handler subscribed to target, the way the
// broker's receive loop would.
func (s *fakeSubscriber) deliver(target string, metadata map[string]string) error {
	GinkgoHelper()

	handler, ok := s.handlers[target]
	Expect(ok).To(BeTrue(), "nothing subscribed to %s", target)
	return handler(context.Background(), []byte(`{}`), metadata)
}

// handlerFunc adapts a function to mb.SubscribeHandler.
type handlerFunc func(ctx context.Context, data []byte, metadata map[string]string) error

func (f handlerFunc) OnReceive(ctx context.Context, data []byte, metadata map[string]string) error {
	return f(ctx, data, metadata)
}
