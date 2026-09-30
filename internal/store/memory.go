package store

import (
	"errors"
	"sync"

	"github.com/zylar06/video-agent/internal/domain"
)

type TimelineStore interface {
	Open(domain.TimelineRevision) error
	Current(string) (domain.TimelineRevision, error)
	Save(domain.TimelineRevision, string) error
	OperationResult(string) (domain.TimelineRevision, bool)
}

type MemoryStore struct {
	mu         sync.RWMutex
	current    map[string]domain.TimelineRevision
	operations map[string]domain.TimelineRevision
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{current: map[string]domain.TimelineRevision{}, operations: map[string]domain.TimelineRevision{}}
}

func (s *MemoryStore) Open(t domain.TimelineRevision) error {
	if t.Revision != 1 {
		return errors.New("new timeline must start at revision 1")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.current[t.ID]; exists {
		return errors.New("timeline already exists")
	}
	s.current[t.ID] = t.Clone()
	return nil
}

func (s *MemoryStore) Current(id string) (domain.TimelineRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.current[id]
	if !ok {
		return domain.TimelineRevision{}, errors.New("timeline not found")
	}
	return t.Clone(), nil
}

func (s *MemoryStore) Save(t domain.TimelineRevision, operationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current[t.ID] = t.Clone()
	if operationID != "" {
		s.operations[operationID] = t.Clone()
	}
	return nil
}

func (s *MemoryStore) OperationResult(id string) (domain.TimelineRevision, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.operations[id]
	if !ok {
		return domain.TimelineRevision{}, false
	}
	return t.Clone(), true
}
