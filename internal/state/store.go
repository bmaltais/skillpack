package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/bmaltais/skillpack/internal/config"
)

// Store is the persistence seam: one place that decides how a State is saved.
// Adapters: the JSON file under ~/.skillpack (default) and MemoryStore (tests).
type Store interface {
	Load() (*State, error)
	Save(st *State) error
}

type fileStore struct{}

// Load reads ~/.skillpack/state.json; a missing file yields an empty State.
func (fileStore) Load() (*State, error) {
	p, err := statePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return empty(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading state: %w", err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}
	st.ensureMaps()
	return &st, nil
}

// Save writes state to ~/.skillpack/state.json.
func (fileStore) Save(st *State) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating skillpack dir: %w", err)
	}
	p := filepath.Join(dir, "state.json")
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	return os.WriteFile(p, data, 0600)
}

// MemoryStore is an in-memory Store that records snapshots instead of writing
// to disk, so tests need no temp HOME.
type MemoryStore struct {
	mu    sync.Mutex
	saves int
	last  *State
}

// New returns an empty State bound to the JSON file store.
func New() *State { return empty() }

// NewMemory returns an empty State bound to a fresh MemoryStore.
func NewMemory() (*State, *MemoryStore) {
	mem := &MemoryStore{}
	st := empty()
	st.store = mem
	return st, mem
}

// NewMemoryFrom returns a deep copy of seed bound to a fresh MemoryStore, so
// tests can start from a populated State literal without a temp HOME.
func NewMemoryFrom(seed *State) (*State, *MemoryStore) {
	mem := &MemoryStore{}
	st := seed.Clone()
	st.store = mem
	return st, mem
}

// Save records a deep snapshot of st.
func (m *MemoryStore) Save(st *State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saves++
	m.last = st.Clone()
	return nil
}

// Load returns a clone of the last saved snapshot bound to this store, or an
// empty State if nothing was saved yet.
func (m *MemoryStore) Load() (*State, error) {
	m.mu.Lock()
	last := m.last
	m.mu.Unlock()
	var st *State
	if last == nil {
		st = empty()
	} else {
		st = last.Clone()
	}
	st.store = m
	return st, nil
}

// Saves returns how many times Save was called.
func (m *MemoryStore) Saves() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saves
}

// Last returns the most recently saved snapshot, or nil if never saved.
func (m *MemoryStore) Last() *State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last
}

// Clone returns a deep copy that saves through the same store. Async commands
// use it to avoid data races with UI rendering. The copy is a JSON round-trip,
// so every exported field with a json tag is copied without listing it here.
func (st *State) Clone() *State {
	data, err := json.Marshal(st)
	if err != nil {
		panic(fmt.Sprintf("state: cloning: %v", err)) // State is plain data; cannot fail
	}
	var dst State
	if err := json.Unmarshal(data, &dst); err != nil {
		panic(fmt.Sprintf("state: cloning: %v", err))
	}
	dst.ensureMaps()
	dst.store = st.store
	return &dst
}

// ensureMaps replaces nil top-level maps with empty ones.
func (st *State) ensureMaps() {
	if st.Repos == nil {
		st.Repos = make(map[string]RepoRecord)
	}
	if st.InstalledSkills == nil {
		st.InstalledSkills = make(map[string]map[string]InstalledSkillRecord)
	}
	if st.InstalledPacks == nil {
		st.InstalledPacks = make(map[string]InstalledPackRecord)
	}
}
