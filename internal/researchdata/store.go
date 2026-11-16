package researchdata

// Store is the minimal versioned-write/read interface used by the QuantBrew registry importer.
type Store interface {
	PutVersioned(logicalKey string, payload []byte, source string) (seq uint64, err error)
	GetLatest(logicalKey string) (payload []byte, found bool, err error)
}

// EngineSource is retained for the audit trail of previously imported records.
const EngineSource = "jobctl"
