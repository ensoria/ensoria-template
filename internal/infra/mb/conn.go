package mb

import (
	"context"
	"errors"
	"fmt"

	"github.com/ensoria/config/pkg/registry"
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

// BrokerConfig はこの環境で使うメッセージブローカーの接続設定をconfigから組み立てる。
//
// SubscriberとPublisher、そしてdescribe（AsyncAPI生成）の3者が同じ値を見るように、
// 設定はここ1箇所だけで組み立てる。接続だけが知っていてドキュメントが知らない
// ブローカーがあると、生成物は実際の接続先とずれる。
//
// ブローカーが設定されていない（BROKER_TYPEが空）場合はnilを返す。発行も購読も
// しないアプリは存在するので、未設定はエラーではない。
func BrokerConfig() *enmb.Config {
	params, err := registry.ModuleParams("default")
	if err != nil || !params.Broker.Configured() {
		return nil
	}

	config := &enmb.Config{
		Type:      enmb.BrokerType(params.Broker.Type),
		URL:       params.Broker.URL,
		QueueName: params.Broker.QueueName,
	}
	// 資格情報は設定されているときだけ載せる。認証を取らないブローカーと、
	// 誰も設定していないブローカーを区別できるようにするため。
	if params.Broker.HasCredentials() || params.Broker.SASLMechanism != "" {
		config.Credentials = &enmb.Credentials{
			Username:      params.Broker.Username,
			Password:      params.Broker.Password,
			Token:         params.Broker.Token,
			SASLMechanism: params.Broker.SASLMechanism,
		}
	}
	return config
}

// errNoBroker は、購読や発行を行うのにブローカーが設定されていない場合のエラー。
// 起動時に明示的に失敗させる —— 接続できないまま起動すると、購読しているはずの
// チャンネルを黙って購読しないアプリになる。
var errNoBroker = errors.New("message broker is not configured: set BROKER_TYPE (and BROKER_URL) in the environment configuration")

func NewSubscriberConnection(envVal *string) func(lc dikit.LC) (enmb.Subscriber, error) {
	return func(lc dikit.LC) (enmb.Subscriber, error) {
		config := BrokerConfig()
		if config == nil {
			return nil, errNoBroker
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
		config := BrokerConfig()
		if config == nil {
			return nil, errNoBroker
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
