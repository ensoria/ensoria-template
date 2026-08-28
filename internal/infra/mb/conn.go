package mb

import (
	"context"
	"errors"
	"fmt"

	"github.com/ensoria/config/pkg/appconfig"
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
// A missing broker configuration (an empty BROKER_TYPE) is not an error and
// yields (nil, nil): an application that neither publishes nor subscribes is a
// normal thing to run. A configuration that cannot be read at all is an error,
// and a separate one: starting without it would silently produce an application
// that subscribes to nothing and publishes nothing.
func BrokerConfig() (*enmb.Config, error) {
	params, err := registry.ModuleParams("default")
	if err != nil {
		return nil, fmt.Errorf("broker configuration unavailable: %w", err)
	}
	return brokerConfigFromParams(params), nil
}

// brokerConfigFromParams builds the broker configuration from parameters that
// are already resolved. It is split from BrokerConfig so that the assembly
// rules — unconfigured yields nil, credentials are carried only when present —
// can be tested without any registry state.
func brokerConfigFromParams(params *appconfig.Parameters) *enmb.Config {
	if !params.Broker.Configured() {
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
		config, err := BrokerConfig()
		if err != nil {
			return nil, err
		}
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
		config, err := BrokerConfig()
		if err != nil {
			return nil, err
		}
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
