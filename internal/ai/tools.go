package ai

import (
	"bytes"
	"encoding/json"
)

// The JSON a tool result is written as.

// EncodeToolOutput encodes a tool result as the JSON text a model reads.
func EncodeToolOutput(answered any) string {
	written := &bytes.Buffer{}
	encoder := json.NewEncoder(written)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(answered); err != nil {
		return `{"error":"cannot encode the tool result as JSON"}`
	}
	return string(bytes.TrimRight(written.Bytes(), "\n"))
}
