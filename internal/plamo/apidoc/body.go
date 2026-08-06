package apidoc

import (
	"reflect"

	"github.com/ensoria/validator/pkg/rule"
)

// BodySchema builds the documentation schema of one payload type: the schema
// tree from the type, the declared validation rules as constraints, the declared
// field meanings, and a deterministic example on the root node.
//
// It exists so that every surface describing a payload goes through the same
// steps — HTTP request and response bodies here, and the message payloads
// msgdoc describes. Leaving one of those steps out (the example, say) is what
// makes two generators disagree about the very same Go type.
//
// docs are applied in order, so a later map wins over an earlier one for the
// same key: pass the shared declaration first and the side-specific one second.
//
// Returns nil when the type carries no body at all: a nil type, or a struct with
// no exported fields (restkit.NoBody).
func BodySchema(t reflect.Type, rules []*rule.RuleSet, opts ExampleOptions, docs ...map[string]string) *Schema {
	schema := SchemaFromType(t)
	if schema == nil {
		return nil
	}
	applyRules(schema, rules)
	for _, d := range docs {
		for path, meaning := range d {
			setFieldMeaning(schema, path, meaning)
		}
	}
	schema.Example = ExampleFromType(t, rules, opts)
	return schema
}
