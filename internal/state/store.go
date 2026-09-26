package state

// Store is the persistence seam: one place that decides how a State is saved.
// Adapters: the JSON file under ~/.skillpack (default) and MemoryStore (tests).
type Store interface {
	Save(st *State) error
}

type fileStore struct{}

// MemoryStore is an in-memory Store that records snapshots instead of writing
// to disk, so tests need no temp HOME.
type MemoryStore struct {
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

// Save records a deep snapshot of st.
func (m *MemoryStore) Save(st *State) error {
	m.saves++
	m.last = st.Clone()
	return nil
}

// Saves returns how many times Save was called.
func (m *MemoryStore) Saves() int { return m.saves }

// Last returns the most recently saved snapshot, or nil if never saved.
func (m *MemoryStore) Last() *State { return m.last }

// Clone returns a deep copy that saves through the same store. Async commands
// use it to avoid data races with UI rendering.
func (st *State) Clone() *State {
	dst := &State{
		Repos:           make(map[string]RepoRecord, len(st.Repos)),
		InstalledSkills: make(map[string]map[string]InstalledSkillRecord, len(st.InstalledSkills)),
		InstalledPacks:  make(map[string]InstalledPackRecord, len(st.InstalledPacks)),
		store:           st.store,
	}
	for k, v := range st.Repos {
		dst.Repos[k] = v
	}
	for addr, agents := range st.InstalledSkills {
		dst.InstalledSkills[addr] = make(map[string]InstalledSkillRecord, len(agents))
		for agent, rec := range agents {
			dst.InstalledSkills[addr][agent] = rec
		}
	}
	for packAddr, rec := range st.InstalledPacks {
		newRec := rec
		newRec.Agents = append([]string{}, rec.Agents...)
		newRec.Skills = make(map[string]map[string]PackSkillStatus, len(rec.Skills))
		for skillAddr, agStatuses := range rec.Skills {
			newRec.Skills[skillAddr] = make(map[string]PackSkillStatus, len(agStatuses))
			for agName, s := range agStatuses {
				newRec.Skills[skillAddr][agName] = s
			}
		}
		dst.InstalledPacks[packAddr] = newRec
	}
	return dst
}
