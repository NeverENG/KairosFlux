package jobctl

import "sync"

type fakeStore struct {
	mu       sync.Mutex
	versions map[string][][]byte
}

func newFakeStore() *fakeStore { return &fakeStore{versions: make(map[string][][]byte)} }

func (s *fakeStore) PutVersioned(key string, payload []byte, _ string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.versions[key] = append(s.versions[key], append([]byte(nil), payload...))
	return uint64(len(s.versions[key])), nil
}

func (s *fakeStore) GetLatest(key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vs := s.versions[key]
	if len(vs) == 0 {
		return nil, false, nil
	}
	return vs[len(vs)-1], true, nil
}

func (s *fakeStore) versionCount(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.versions[key])
}
