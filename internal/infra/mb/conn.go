package mb

import (
	"context"
	"fmt"

	"github.com/ensoria/ensoria-template/internal/plamo/dikit"
	"github.com/ensoria/loggear/pkg/loggear"
	enmb "github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/mb/pkg/mq"
)

// message brokerに関する接続

type SubscriberPanicHandler struct{}

func (h *SubscriberPanicHandler) OnPanic(panicValue interface{}, stackTrace []byte, metadata enmb.PanicMetadata) {
	loggear.Error("Panic Recovered in Subscriber",
		"target", metadata.Target,
		"metadata", metadata.Metadata,
		"data", metadata.Data,
		"panic_value", panicValue,
		"panic_type", fmt.Sprintf("%T", panicValue),
		"stack_trace", string(stackTrace),
		"type", "subscriber_panic_log",
	)
}

func NewSubscriberConnection(envVal *string) func(lc dikit.LC) (enmb.Subscriber, error) {
	return func(lc dikit.LC) (enmb.Subscriber, error) {
		// TODO: envValを使って、その環境の値をconfigから取得するようにする
		// configにはメッセージブローカーの実装がないので、configで実装してから変更
		config := &enmb.Config{
			Type: enmb.TypeRabbitMQ,
			URL:  "amqp://localhost:5672/",
			Credentials: &enmb.Credentials{
				Username: "myuser",
				Password: "mypassword",
			},
		}

		subConn, err := mq.NewSubscriber(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create subscriber: %w", err)
		}

		subConn.SetOptions(
			enmb.WithLogger(loggear.GetLogger()),
			enmb.WithPanicHandler(&SubscriberPanicHandler{}),
		)

		lc.Append(dikit.Hook{
			OnStart: func(ctx context.Context) error {
				if err := subConn.Connect(ctx); err != nil {
					return fmt.Errorf("MB subscriber connection failed: %w", err)
				}
				if err := subConn.Ping(ctx); err != nil {
					return fmt.Errorf("MB subscriber connection check failed: %w", err)
				}
				loggear.Info("MB subscriber connection verified")
				return nil
			},
			OnStop: func(ctx context.Context) error {
				loggear.Info("Shutting down MB subscriber")
				return subConn.Close()
			},
		})

		return subConn, nil
	}
}

func NewPublisherConnection(envVal *string) func(lc dikit.LC) (enmb.Publisher, error) {
	return func(lc dikit.LC) (enmb.Publisher, error) {
		// configから取得する
		config := &enmb.Config{
			Type: enmb.TypeRabbitMQ,
			URL:  "amqp://localhost:5672/",
			Credentials: &enmb.Credentials{
				Username: "myuser",
				Password: "mypassword",
			},
		}

		pubConn, err := mq.NewPublisher(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create publisher: %w", err)
		}

		pubConn.SetOptions(enmb.WithPublishLogger(loggear.GetLogger()))

		lc.Append(dikit.Hook{
			OnStart: func(ctx context.Context) error {
				if err := pubConn.Connect(ctx); err != nil {
					return fmt.Errorf("MB publisher connection failed: %w", err)
				}
				if err := pubConn.Ping(ctx); err != nil {
					return fmt.Errorf("MB publisher connection check failed: %w", err)
				}
				loggear.Info("MB publisher connection verified")
				return nil
			},
			OnStop: func(ctx context.Context) error {
				loggear.Info("Shutting down MB publisher")
				return pubConn.Close()
			},
		})

		return pubConn, nil
	}
}
