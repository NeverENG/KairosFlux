package researchdata

import (
	"encoding/json"
	"strings"
	"testing"
)

type sourceCaptureStore struct {
	*fakeStore
	source string
}

func (s *sourceCaptureStore) PutVersioned(key string, payload []byte, source string) (uint64, error) {
	s.source = source
	return s.fakeStore.PutVersioned(key, payload, source)
}

// Renaming the Go package must not change persisted keys, source audit records or phase values.
func TestRegistryImportPreservesStoredContract(t *testing.T) {
	store := &sourceCaptureStore{fakeStore: newFakeStore()}
	line := `{"verdict":{"fingerprint":"factor-1","level":"pass_raw"}}`
	result, err := (&RegistryImporter{Store: store}).ImportReader(strings.NewReader(line + "\n"))
	if err != nil || result.Imported != 1 {
		t.Fatalf("import: result=%+v err=%v", result, err)
	}
	if store.source != "jobctl" {
		t.Fatalf("persisted audit source changed: %q", store.source)
	}
	key := "strategy:index:factor-1"
	if RegistryIndexKey("factor-1") != key || RegistryIndexPrefix() != "strategy:index:" {
		t.Fatalf("persisted registry key changed: %q", RegistryIndexKey("factor-1"))
	}
	raw, found, err := store.GetLatest(key)
	if err != nil || !found || string(raw) != line {
		t.Fatalf("persisted registry payload changed: found=%v raw=%q err=%v", found, raw, err)
	}
}

func TestStrategyPhaseSerializedValues(t *testing.T) {
	phases := []StrategyPhase{StrategyHypothesis, StrategyGate, StrategyCandidate, StrategyPaper, StrategyLive, StrategyRetired}
	got, err := json.Marshal(phases)
	if err != nil {
		t.Fatal(err)
	}
	want := `["hypothesis","gate","candidate","paper","live","retired"]`
	if string(got) != want {
		t.Fatalf("strategy phase encoding changed: got %s want %s", got, want)
	}
}
