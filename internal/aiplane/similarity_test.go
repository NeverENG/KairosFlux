package aiplane

import (
	"math"
	"testing"
)

func TestStoreSimilarityPreservesLegacyEdgeAndIsIdempotent(t *testing.T) {
	store := newFakeReadWriter()
	input := SimilarityEdge{FactorA: "factorB", FactorB: "factorA", Correlation: 0.95, SuspectDuplicate: true}
	for i := 0; i < 2; i++ {
		if _, err := StoreSimilarity(store, input); err != nil {
			t.Fatal(err)
		}
	}
	key := "factor:similarity:factorA|factorB"
	if count := store.versionCount(key); count != 1 {
		t.Fatalf("duplicate write created %d versions", count)
	}
	raw, found, err := store.GetAsOf(key, store.currentClock())
	want := `{"factor_a":"factorA","factor_b":"factorB","correlation":0.95,"suspect_duplicate":true}`
	if err != nil || !found || string(raw) != want {
		t.Fatalf("legacy edge format changed: found=%v raw=%s err=%v", found, raw, err)
	}
	got, found, err := LookupSimilarity(store, "factorA", "factorB", store.currentClock())
	if err != nil || !found || got.SuspectDuplicate != input.SuspectDuplicate || got.Correlation != input.Correlation {
		t.Fatalf("lookup: found=%v edge=%+v err=%v", found, got, err)
	}
}

func TestStoreSimilarityRejectsInvalidShape(t *testing.T) {
	for _, input := range []SimilarityEdge{
		{FactorA: "", FactorB: "factorB", Correlation: 0.5},
		{FactorA: "same", FactorB: "same", Correlation: 0.5},
		{FactorA: "a", FactorB: "b", Correlation: math.NaN()},
		{FactorA: "a", FactorB: "b", Correlation: 1.5},
	} {
		store := newFakeReadWriter()
		if _, err := StoreSimilarity(store, input); err == nil {
			t.Fatalf("invalid similarity accepted: %+v", input)
		}
		if store.currentClock() != 0 {
			t.Fatalf("invalid similarity wrote data: %+v", input)
		}
	}
}

func TestLinkFactorToExperimentDoesNotDecideAdmission(t *testing.T) {
	store := newFakeReadWriter()
	_, err := StoreSimilarity(store, SimilarityEdge{FactorA: "factorA", FactorB: "factorB", Correlation: 0.95, SuspectDuplicate: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := LinkFactorToExperiment(store, "factorA", "expA"); err != nil {
		t.Fatal(err)
	}
	if err := LinkFactorToExperiment(store, "factorB", "expB"); err != nil {
		t.Fatalf("database rejected upstream decision to link factorB: %v", err)
	}
	raw, found, err := store.GetAsOf("evidence:factor:factorB:expB", store.currentClock())
	if err != nil || !found || string(raw) != `{"from":"factorB","to":"expB"}` {
		t.Fatalf("factor evidence changed: found=%v raw=%s err=%v", found, raw, err)
	}
}
