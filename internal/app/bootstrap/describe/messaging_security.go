package describe

import (
	"net/url"

	"github.com/ensoria/config/pkg/appconfig"
	"github.com/ensoria/ensoria-template/internal/plamo/authkit"
	"github.com/ensoria/ensoria-template/internal/plamo/msgdoc"
	"github.com/ensoria/mb/pkg/mb"
)

// The SASL mechanisms mb understands, spelt as BROKER_SASL_MECHANISM takes them.
// No mechanism means PLAIN, as it does in mb.
const (
	saslPlain       = "PLAIN"
	saslScramSHA256 = "SCRAM-SHA-256"
	saslScramSHA512 = "SCRAM-SHA-512"
)

// brokerSecurity names the kind of credential the broker is reached with, so
// that a team wanting to produce to it learns how to connect from the document
// alone. Only the kind is written, never the credential: a generated document
// is committed and shared.
//
// The kinds follow what mb does with the settings, not what the settings look
// like: Kafka with no mechanism named authenticates with PLAIN, SASL is used
// only with both a user name and a password, and NATS prefers a user name and
// password over a token.
func brokerSecurity(broker *mb.Config) []*msgdoc.ServerSecurity {
	if broker == nil {
		return nil
	}
	creds := broker.Credentials
	if creds == nil {
		creds = &mb.Credentials{}
	}

	switch broker.Type {
	case mb.TypeSQS, mb.TypeKinesis:
		return serverSecurity(msgdoc.ServerAuthAWSIAM, "AWS credentials from the environment, authorized by an IAM policy")
	case mb.TypeRabbitMQ:
		if creds.Username != "" || urlHasUser(broker.URL) {
			return serverSecurity(msgdoc.ServerAuthUserPassword, "A user name and password")
		}
	case mb.TypeNATS:
		switch {
		case creds.Username != "" && creds.Password != "", urlHasUser(broker.URL):
			return serverSecurity(msgdoc.ServerAuthUserPassword, "A user name and password")
		case creds.Token != "":
			return serverSecurity(msgdoc.ServerAuthToken, "A token presented to the broker")
		}
	case mb.TypeKafka, mb.TypeRedpanda:
		if creds.Username == "" || creds.Password == "" {
			return nil
		}
		switch creds.SASLMechanism {
		case "", saslPlain:
			return serverSecurity(msgdoc.ServerAuthPlain, "SASL PLAIN with a user name and password")
		case saslScramSHA256:
			return serverSecurity(msgdoc.ServerAuthScramSHA256, "SASL SCRAM-SHA-256 with a user name and password")
		case saslScramSHA512:
			return serverSecurity(msgdoc.ServerAuthScramSHA512, "SASL SCRAM-SHA-512 with a user name and password")
		}
		// Any other mechanism is one mb refuses to start with, so there is no
		// running application for the document to describe.
	}
	return nil
}

// serverSecurity is the one-entry list every broker kind above produces.
func serverSecurity(kind, description string) []*msgdoc.ServerSecurity {
	return []*msgdoc.ServerSecurity{{Kind: kind, Description: description}}
}

// urlHasUser reports whether a broker URL carries a user of its own
// ("amqp://user:pass@host").
func urlHasUser(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.User != nil && parsed.User.Username() != ""
}

// verifiableSchemes names the kinds of credential the application verifies —
// what a WebSocket channel accepts when it limits itself to none. They are the
// names of the security schemes the conventions list. params is nil when the
// configuration could not be read, and then nothing is named.
func verifiableSchemes(params *appconfig.Parameters) []string {
	if params == nil {
		return nil
	}
	return authkit.ConfiguredSchemes(params.Auth)
}
