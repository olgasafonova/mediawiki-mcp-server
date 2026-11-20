// Package tools provides a metadata-driven registry for MCP tool definitions.
// It reduces boilerplate in main.go by defining tools declaratively and
// using type-safe handlers to register them.
package tools

// ToolSpec defines a tool's metadata for declarative registration.
// Each spec maps to a wiki.Client method with matching Args/Result types.
type ToolSpec struct {
	// Name is the MCP tool name (e.g., "mediawiki_search")
	Name string

	// Method is the wiki.Client method name (e.g., "Search")
	Method string

	// Description is the tool description shown to LLMs
	Description string

	// Title is the human-readable tool title for annotations
	Title string

	// Category groups tools logically (search, read, write, etc.)
	Category string

	// ReadOnly indicates the tool doesn't modify wiki state
	ReadOnly bool

	// Destructive indicates the tool can delete or overwrite data. It is
	// emitted as destructiveHint on every non-read-only tool, so false is a
	// real claim (additive only), not "unset".
	Destructive bool

	// Idempotent indicates repeated calls have the same effect
	Idempotent bool

	// OpenWorld indicates the tool accesses external resources. It is always
	// emitted as openWorldHint, so false is a real claim (never leaves the
	// process), not "unset". Every wiki-backed tool must set it to true.
	OpenWorld bool
}

// ptr is a helper to create a pointer to a value.
func ptr[T any](v T) *T {
	return &v
}
