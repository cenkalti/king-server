package protocol

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var jsonBlock = regexp.MustCompile("(?s)```json\n(.*?)```")

// Every JSON example in PROTOCOL.md must validate against the schema and
// decode into our types.
func TestProtocolDocExamplesValidate(t *testing.T) {
	doc, err := os.ReadFile("../PROTOCOL.md")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(SchemaID, schemaDoc); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile(SchemaID)
	if err != nil {
		t.Fatal(err)
	}

	blocks := jsonBlock.FindAllSubmatch(doc, -1)
	if len(blocks) < 7 {
		t.Fatalf("found only %d json examples", len(blocks))
	}
	seen := map[string]bool{}
	for _, b := range blocks {
		v, err := jsonschema.UnmarshalJSON(bytes.NewReader(b[1]))
		if err != nil {
			t.Fatalf("example is not JSON: %v\n%s", err, b[1])
		}
		if err := sch.Validate(v); err != nil {
			t.Fatalf("example does not match the schema: %v\n%s", err, b[1])
		}
		var env Envelope
		json.Unmarshal(b[1], &env)
		seen[env.Type] = true
		if env.Type == TypeRequest {
			var req Request
			if err := json.Unmarshal(b[1], &req); err != nil {
				t.Fatalf("request does not decode: %v", err)
			}
		}
	}
	for _, typ := range []string{TypeHello, TypeWelcome, TypeError, TypeRequest, TypeResponse, TypeMatchEnd} {
		if !seen[typ] {
			t.Errorf("PROTOCOL.md has no %s example", typ)
		}
	}
}

// The schema rejects common mistakes.
func TestSchemaRejects(t *testing.T) {
	raw, _ := os.ReadFile(schemaPath)
	schemaDoc, _ := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	c := jsonschema.NewCompiler()
	c.AddResource(SchemaID, schemaDoc)
	sch := c.MustCompile(SchemaID)
	for _, bad := range []string{
		`{"type":"response","id":"1","card":{"rank":1,"suit":"S"}}`,
		`{"type":"response","id":"1","card":{"rank":12,"suit":"X"}}`,
		`{"type":"response","id":"1","contract":{"kind":"rifki"}}`,
		`{"type":"hello"}`,
		`{"type":"helo","name":"a"}`,
	} {
		v, _ := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(bad)))
		if sch.Validate(v) == nil {
			t.Errorf("schema accepted %s", bad)
		}
	}
}
