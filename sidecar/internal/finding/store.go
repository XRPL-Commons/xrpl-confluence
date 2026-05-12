package finding

import (
	"sort"
	"sync"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

const maxCapacity = 1000

// ListOpts filters the result set returned by Store.List.
type ListOpts struct {
	// Since is a finding ID; only findings with OpenedAt strictly after
	// that finding's OpenedAt are returned.
	Since string
	// Kind filters by finding kind; ignored when empty.
	Kind  string
	// Limit caps the number of results (default 100, max 1000).
	Limit int
}

// Store is a thread-safe in-memory ring buffer of api.Finding records.
type Store struct {
	mu       sync.RWMutex
	findings []api.Finding
	byID     map[string]int // id → index in findings

	onAdd func(api.Finding)
}

func NewStore() *Store {
	return &Store{byID: make(map[string]int)}
}

// SetOnAdd registers a callback invoked (outside the store's lock) each time a
// finding is added. It replaces any previously registered callback.
func (s *Store) SetOnAdd(fn func(api.Finding)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onAdd = fn
}

// Add appends f to the store. If the store is at capacity the oldest finding
// is evicted first. Any registered SetOnAdd callback is invoked after the
// lock is released.
func (s *Store) Add(f api.Finding) {
	s.mu.Lock()

	if len(s.findings) >= maxCapacity {
		oldest := s.findings[0]
		delete(s.byID, oldest.ID)
		s.findings = s.findings[1:]
		// Shift all indices down by one.
		for id, idx := range s.byID {
			s.byID[id] = idx - 1
		}
	}

	s.byID[f.ID] = len(s.findings)
	s.findings = append(s.findings, f)
	cb := s.onAdd

	s.mu.Unlock()

	if cb != nil {
		cb(f)
	}
}

// GetByID returns the finding with the given ID, or (zero, false) if not found.
func (s *Store) GetByID(id string) (api.Finding, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	idx, ok := s.byID[id]
	if !ok {
		return api.Finding{}, false
	}
	return s.findings[idx], true
}

// Len returns the number of findings currently held.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.findings)
}

// List returns a copy of findings, filtered by opts and ordered newest-first.
func (s *Store) List(opts ListOpts) []api.Finding {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > maxCapacity {
		limit = maxCapacity
	}

	// Resolve the Since marker's OpenedAt outside the copy loop.
	var sinceTime interface{ IsZero() bool }
	type zeroer interface{ IsZero() bool }
	_ = sinceTime
	var sinceSet bool
	var sinceIdx int
	if opts.Since != "" {
		if idx, ok := s.byID[opts.Since]; ok {
			sinceIdx = idx
			sinceSet = true
		}
	}

	// Build result slice (newest-first via reverse iteration).
	result := make([]api.Finding, 0, limit)
	for i := len(s.findings) - 1; i >= 0 && len(result) < limit; i-- {
		f := s.findings[i]
		if opts.Kind != "" && f.Kind != opts.Kind {
			continue
		}
		if sinceSet && !f.OpenedAt.After(s.findings[sinceIdx].OpenedAt) {
			continue
		}
		result = append(result, f)
	}

	// Stable sort in case multiple findings share the same OpenedAt.
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].OpenedAt.After(result[j].OpenedAt)
	})

	return result
}
