package protocol

//go:generate go run ./internal/genschema ../schema/king.schema.json

import (
	"encoding/json"
	"maps"
	"reflect"

	"github.com/invopop/jsonschema"

	"github.com/cenkalti/king-server/king"
)

// SchemaID is the $id of the generated schema.
const SchemaID = "https://github.com/cenkalti/king-server/schema/king.schema.json"

// Messages lists every wire message type, in the order they appear in the
// schema's top-level oneOf.
var Messages = []any{Hello{}, Welcome{}, Error{}, Request{}, Response{}, MatchEnd{}}

func suitSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Enum: []any{"C", "D", "H", "S"}, Description: "Clubs, Diamonds, Hearts, Spades."}
}

// mapper points king types with custom JSON encoding at hand-written
// definitions, so they appear once in $defs and get proper names in
// generated code.
func mapper(t reflect.Type) *jsonschema.Schema {
	switch t {
	case reflect.TypeFor[king.Suit]():
		return &jsonschema.Schema{Ref: "#/$defs/Suit"}
	case reflect.TypeFor[king.Contract]():
		return &jsonschema.Schema{Ref: "#/$defs/Contract"}
	}
	return nil
}

func contractSchema() *jsonschema.Schema {
	kinds := make([]any, 0, 7)
	for _, n := range king.KindNames() {
		kinds = append(kinds, n)
	}
	props := jsonschema.NewProperties()
	props.Set("kind", &jsonschema.Schema{Type: "string", Enum: kinds})
	props.Set("suit", &jsonschema.Schema{Ref: "#/$defs/Suit", Description: "Trump suit. Present only when kind is trump."})
	return &jsonschema.Schema{
		Type:                 "object",
		Properties:           props,
		Required:             []string{"kind"},
		AdditionalProperties: jsonschema.FalseSchema,
	}
}

// Schema returns the JSON Schema document for all messages, indented and
// newline-terminated. It must run with the protocol package directory as the
// working directory (as go generate and go test do) so Go doc comments can be
// copied into descriptions.
func Schema() ([]byte, error) {
	r := &jsonschema.Reflector{Mapper: mapper}
	if err := r.AddGoComments("github.com/cenkalti/king-server/protocol", "."); err != nil {
		return nil, err
	}
	if err := r.AddGoComments("github.com/cenkalti/king-server/king", "../king"); err != nil {
		return nil, err
	}
	root := &jsonschema.Schema{
		Version:     "https://json-schema.org/draft/2020-12/schema",
		ID:          SchemaID,
		Title:       "King bot protocol",
		Description: "Every WebSocket message between the King server and a bot. See PROTOCOL.md.",
		Definitions: jsonschema.Definitions{"Suit": suitSchema(), "Contract": contractSchema()},
	}
	for _, m := range Messages {
		s := r.ReflectFromType(reflect.TypeOf(m))
		maps.Copy(root.Definitions, s.Definitions)
		root.OneOf = append(root.OneOf, &jsonschema.Schema{Ref: s.Ref})
	}
	// Titles make code generators such as quicktype use the Go type names.
	for name, def := range root.Definitions {
		def.Title = name
	}
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
