package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Spittingjiu/sui-go/internal/model"
)

type Store struct {
	mu       sync.RWMutex
	dataFile string
	nextID   int64
	items    []model.Inbound
}

func New(dataFile string) (*Store, error) {
	s := &Store{dataFile: dataFile, nextID: 1}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := os.ReadFile(s.dataFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, &s.items); err != nil {
		return err
	}
	var maxID int64
	for _, it := range s.items {
		if it.ID > maxID {
			maxID = it.ID
		}
	}
	s.nextID = maxID + 1
	return nil
}

func (s *Store) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.dataFile), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.dataFile, b, 0o644)
}

func (s *Store) List() []model.Inbound {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Inbound, len(s.items))
	copy(out, s.items)
	return out
}

func (s *Store) Get(id int64) (model.Inbound, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, it := range s.items {
		if it.ID == id {
			return it, true
		}
	}
	return model.Inbound{}, false
}

func (s *Store) Add(in model.Inbound) (model.Inbound, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	in.ID = s.nextID
	s.nextID++
	in.Enable = true
	in.CreateUnix = now
	in.UpdateUnix = now
	s.items = append(s.items, in)
	if err := s.persistLocked(); err != nil {
		return model.Inbound{}, err
	}
	return in, nil
}
