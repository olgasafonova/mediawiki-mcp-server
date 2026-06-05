package tools

import "testing"

// TestPtr verifies the generic ptr helper returns a non-nil pointer that
// dereferences to the supplied value, across the value types ToolSpec
// registration relies on (bool for the annotation hints, string for names).
func TestPtr(t *testing.T) {
	t.Run("bool true", func(t *testing.T) {
		p := ptr(true)
		if p == nil {
			t.Fatal("ptr(true) returned nil")
		}
		if *p != true {
			t.Errorf("*ptr(true) = %v, want true", *p)
		}
	})

	t.Run("bool false", func(t *testing.T) {
		p := ptr(false)
		if p == nil {
			t.Fatal("ptr(false) returned nil")
		}
		if *p != false {
			t.Errorf("*ptr(false) = %v, want false", *p)
		}
	})

	t.Run("string", func(t *testing.T) {
		p := ptr("mediawiki_search")
		if p == nil {
			t.Fatal("ptr(string) returned nil")
		}
		if *p != "mediawiki_search" {
			t.Errorf("*ptr(string) = %q, want %q", *p, "mediawiki_search")
		}
	})

	t.Run("int", func(t *testing.T) {
		p := ptr(42)
		if p == nil {
			t.Fatal("ptr(int) returned nil")
		}
		if *p != 42 {
			t.Errorf("*ptr(int) = %d, want 42", *p)
		}
	})
}

// TestPtrDistinctAddresses verifies each ptr call allocates a fresh value so
// mutating one pointer never aliases another. Annotation hints are built with
// repeated ptr(true)/ptr(false) calls and must not share storage.
func TestPtrDistinctAddresses(t *testing.T) {
	a := ptr(true)
	b := ptr(true)
	if a == b {
		t.Fatal("ptr returned the same address for two calls; values would alias")
	}
	*a = false
	if *b != true {
		t.Error("mutating one ptr result changed another; storage is shared")
	}
}

// TestToolSpecZeroValue documents the zero value of ToolSpec: an unset spec is
// read-only=false and non-destructive=false, the conservative defaults the
// registry assigns before a category sets explicit annotations.
func TestToolSpecZeroValue(t *testing.T) {
	var spec ToolSpec
	if spec.Name != "" || spec.Method != "" {
		t.Errorf("zero ToolSpec has non-empty identifiers: %+v", spec)
	}
	if spec.ReadOnly || spec.Destructive || spec.Idempotent || spec.OpenWorld {
		t.Errorf("zero ToolSpec has a true annotation hint: %+v", spec)
	}
}

// TestAllToolsRegistryIntegrity verifies the registry data registry.go's
// ToolSpec drives is well formed: every tool has a name, a method, and a
// description, and no two tools share a name (duplicate names would shadow each
// other at registration).
func TestAllToolsRegistryIntegrity(t *testing.T) {
	if len(AllTools) == 0 {
		t.Fatal("AllTools is empty")
	}

	seen := make(map[string]bool, len(AllTools))
	for i, spec := range AllTools {
		if spec.Name == "" {
			t.Errorf("AllTools[%d] has empty Name", i)
		}
		if spec.Method == "" {
			t.Errorf("AllTools[%d] (%s) has empty Method", i, spec.Name)
		}
		if spec.Description == "" {
			t.Errorf("AllTools[%d] (%s) has empty Description", i, spec.Name)
		}
		if seen[spec.Name] {
			t.Errorf("duplicate tool name %q in AllTools", spec.Name)
		}
		seen[spec.Name] = true
	}
}
