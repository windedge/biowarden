package session

// MemoryStore is the in-memory Store: the tests' stand-in for the Credential
// Manager, so no test touches the real one. The error fields and the counters
// exist so a caller's test can drive a failing store and assert what it wrote.
type MemoryStore struct {
	Rec Record
	Has bool

	SaveErr  error
	LoadErr  error
	ClearErr error

	Saves  int
	Clears []string
}

func (s *MemoryStore) Save(rec Record) error {
	s.Saves++
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.Rec, s.Has = rec, true
	return nil
}

func (s *MemoryStore) Load() (Record, error) {
	if s.LoadErr != nil {
		return Record{}, s.LoadErr
	}
	if !s.Has {
		return Record{}, ErrNoStoredSession
	}
	return s.Rec, nil
}

func (s *MemoryStore) Clear(userID string) error {
	s.Clears = append(s.Clears, userID)
	if s.ClearErr != nil {
		return s.ClearErr
	}
	if s.Has && s.Rec.UserID == userID {
		s.Has = false
	}
	return nil
}
