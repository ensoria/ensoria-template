package describe

import (
	"reflect"
	"strings"
	"time"

	"github.com/ensoria/ensoria-template/internal/plamo/apidoc"
	"github.com/ensoria/ensoria-template/internal/plamo/initiator"
	"github.com/ensoria/ensoria-template/internal/plamo/msgdoc"
	"github.com/ensoria/ensoria-template/internal/plamo/wskit"
	"github.com/ensoria/validator/pkg/rule"
)

// strAnyOf is the rule the kind header is documented with. Nothing is
// validated with it, so it carries no messages.
var strAnyOf = rule.CreateStrAnyOf(nil)

// initiatorHeaderSchema describes the headers every broker message carries.
//
// None of them is required. In AsyncAPI one message definition serves both the
// send and the receive side of a channel, and while this application writes
// ce_authtype on everything it publishes, other producers need not — so the
// first is said in the description and not by making the header required.
//
// claimNames are the claims this application carries, which is also exactly
// what it reads: received claims are filtered by the same list.
func initiatorHeaderSchema(claimNames []string) *apidoc.Schema {
	kinds := make([]any, 0, len(initiator.Kinds()))
	for _, k := range initiator.Kinds() {
		kinds = append(kinds, string(k))
	}
	rules := []*rule.RuleSet{{Field: initiator.HeaderKind, Rules: []rule.Rule{strAnyOf(kinds...)}}}
	return apidoc.BodySchema(reflect.TypeFor[initiator.WireHeaders](), rules, apidoc.ExampleOptions{}, map[string]string{
		initiator.HeaderKind: "What kind of party started the work (the CloudEvents authtype values). " +
			"This application writes it on every message it publishes; other producers may leave it out.",
		initiator.HeaderID: "Who, within that kind: an account's or an API key's identifier. " +
			"Absent when nobody is known.",
		initiator.HeaderClaims: claimsMeaning(claimNames),
	})
}

// claimsMeaning explains the claims header, naming the claims carried.
func claimsMeaning(claimNames []string) string {
	const lead = "Attributes of the party that started the work, as a JSON object. "
	if len(claimNames) == 0 {
		return lead + "This application carries none, and on receipt reads none: any it is sent are dropped."
	}
	return lead + "This application carries only these: " + strings.Join(claimNames, ", ") +
		". On receipt it reads only these names, and any other is dropped."
}

// initiatorConvention says once, for the whole surface, what the headers are
// and are not.
func initiatorConvention() *msgdoc.InitiatorConvention {
	return &msgdoc.InitiatorConvention{
		Headers: []string{initiator.HeaderKind, initiator.HeaderID, initiator.HeaderClaims},
		Description: "Every message this application publishes says who started the work it belongs to. " +
			"The headers are information, not a credential: this application does not verify them on receipt, " +
			"and nothing is granted on their strength — whether a producer may publish at all is decided by the broker's access control. " +
			"They state who started the work as of the moment the message was published. " +
			"The names and meanings are those of the CloudEvents authcontext extension (authtype, authid, authclaims), " +
			"but the messages themselves are not CloudEvents.",
	}
}

// websocketConvention states what holds for every WebSocket channel.
// recheckInterval is how often a session's or an API key's connection is asked
// about again; zero leaves it unstated.
func websocketConvention(recheckInterval time.Duration) *msgdoc.WebSocketConvention {
	codes := make([]*msgdoc.CodeMeaning, 0, len(wskit.ErrorCodes()))
	for _, c := range wskit.ErrorCodes() {
		codes = append(codes, &msgdoc.CodeMeaning{Code: c.Code, Meaning: c.Meaning})
	}
	closeCodes := make([]*msgdoc.CloseCodeMeaning, 0, len(wskit.CloseCodes()))
	for _, c := range wskit.CloseCodes() {
		closeCodes = append(closeCodes, &msgdoc.CloseCodeMeaning{Code: int(c.Code), Meaning: c.Meaning, ClientAction: c.ClientAction})
	}

	recheck := &msgdoc.RecheckConvention{
		Description: "A connection does not outlive the credential it was opened with. " +
			"A token's connection ends when the token expires. " +
			"A session's or an API key's is asked about again at the interval: it ends once the session or key is gone, " +
			"or once the caller no longer meets the channel's conditions. " +
			"A credential store that cannot be reached ends nothing.",
		CloseCodes: closeCodes,
	}
	if recheckInterval > 0 {
		recheck.Interval = recheckInterval.String()
	}

	return &msgdoc.WebSocketConvention{
		Authentication: "A connection is authenticated once, at the handshake, with the credentials HTTP accepts: " +
			"a browser sends its session cookie, a machine sends a bearer token in Authorization or an API key in its header. " +
			"A handshake is refused before the upgrade, as an ordinary HTTP response in the shared error shape: " +
			"401 when the channel needs a caller and none was verified, 403 when the caller is known but does not meet the channel's conditions. " +
			"Which credentials each channel accepts is stated on its operations.",
		Error: &msgdoc.ErrorMessageConvention{
			MessageType: wskit.ErrorMessageName,
			Payload:     apidoc.CommonErrorSchema(reflect.TypeFor[wskit.ErrorPayload]()),
			Codes:       codes,
		},
		Recheck: recheck,
	}
}
