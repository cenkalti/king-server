// Package schema embeds the generated protocol JSON Schema.
package schema

import _ "embed"

// JSON is the contents of king.schema.json.
//
//go:embed king.schema.json
var JSON []byte
