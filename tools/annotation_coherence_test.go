package tools

import "testing"

// TestAnnotationCoherence guards against a read-only tool also declaring
// idempotentHint. idempotentHint carries meaning only for tools that modify
// state: a read-only tool is trivially repeatable, so asserting idempotence
// on it says nothing and misleads a client reasoning about retry safety.
//
// The check runs over AllTools rather than over grep output because AllTools
// is what HandlerRegistry builds annotations from, and the specs are spread
// across five definitions files.
//
// Scope note: convert_markdown is registered inline in main_httpserver.go
// against the SDK server rather than through this table, so it is outside
// this test's reach. Keep its annotations coherent by hand.
func TestAnnotationCoherence(t *testing.T) {
	var incoherent []string
	for _, spec := range AllTools {
		if spec.ReadOnly && spec.Idempotent {
			incoherent = append(incoherent, spec.Name)
		}
	}

	if len(incoherent) > 0 {
		t.Errorf("%d of %d specs set both ReadOnly and Idempotent; drop Idempotent on read-only specs: %v",
			len(incoherent), len(AllTools), incoherent)
	}
}
