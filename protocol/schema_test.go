package protocol

import (
	"bytes"
	"os"
	"testing"
)

const schemaPath = "../schema/king.schema.json"

// TestSchemaUpToDate fails when the committed schema differs from what the
// structs generate. Fix it with `go generate ./protocol`.
func TestSchemaUpToDate(t *testing.T) {
	want, err := Schema()
	if err != nil {
		t.Fatal(err)
	}
	again, err := Schema()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, again) {
		t.Fatal("schema generation is not deterministic")
	}
	got, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is stale; run `go generate ./protocol`", schemaPath)
	}
}
