package api

import "github.com/danielgtaylor/huma/v2"

// SpecAPI is the API Spec describes, for the tests to hold answers to its
// schemas.
var SpecAPI = specAPI

// OpenToTokens reports whether API tokens may call op at all.
func OpenToTokens(op *huma.Operation) bool {
	_, open := op.Metadata[tokenScopeKey]
	return open
}

// ToolKey is the key of a tool's name in its operation's Metadata.
const ToolKey = toolKey

// HistorySince is the first day a history can be of.
var HistorySince = historySince
