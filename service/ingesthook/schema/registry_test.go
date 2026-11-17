package schema

import "testing"

type stubValidator struct{ err error }

func (s stubValidator) Validate(value []byte) error { return s.err }

func TestLookupDescriptor_NoMatchReturnsFalse(t *testing.T) {
	if _, ok := LookupDescriptor([]byte("unregistered:1")); ok {
		t.Fatal("unregistered prefix must not match")
	}
}

func TestLookupDescriptor_MatchesRegisteredPrefix(t *testing.T) {
	RegisterDescriptor(Descriptor{KeyPrefix: "stub:", Validation: stubValidator{}})
	t.Cleanup(func() { Unregister("stub:") })

	d, ok := LookupDescriptor([]byte("stub:abc"))
	if !ok || d.Validation == nil {
		t.Fatalf("registered prefix not found: ok=%v descriptor=%+v", ok, d)
	}
}

func TestLookupDescriptor_LongestPrefixWins(t *testing.T) {
	RegisterDescriptor(Descriptor{KeyPrefix: "a:", Name: "short", Validation: stubValidator{}})
	RegisterDescriptor(Descriptor{KeyPrefix: "a:b:", Name: "long", Validation: stubValidator{}})
	t.Cleanup(func() {
		Unregister("a:")
		Unregister("a:b:")
	})

	d, ok := LookupDescriptor([]byte("a:b:c"))
	if !ok || d.Name != "long" {
		t.Fatalf("expected longest prefix, got ok=%v descriptor=%+v", ok, d)
	}
}

// quote: registers its descriptor at init, even before loading contracts from disk.
func TestLookupDescriptor_QuotePrefixRegisteredByDefault(t *testing.T) {
	d, ok := LookupDescriptor([]byte("quote:2026-08-17:600000"))
	if !ok {
		t.Fatal("quote: prefix must be registered at init")
	}
	if _, isQuote := d.Validation.(QuoteSnapshot); !isQuote {
		t.Fatalf("expected QuoteSnapshot validator, got %T", d.Validation)
	}
}
