package finding

import (
	"fmt"
	"testing"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

func makeFindings(n int) []api.Finding {
	findings := make([]api.Finding, n)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range findings {
		findings[i] = api.Finding{
			ID:       NewFindingID(),
			Kind:     api.KindStateDivergence,
			Severity: api.SeverityError,
			OpenedAt: base.Add(time.Duration(i) * time.Second),
			Summary:  fmt.Sprintf("finding %d", i),
		}
	}
	return findings
}

func TestStore_Empty(t *testing.T) {
	s := NewStore()
	if s.Len() != 0 {
		t.Fatalf("expected Len 0, got %d", s.Len())
	}
	list := s.List(ListOpts{})
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}
	_, ok := s.GetByID("fnd_nonexistent")
	if ok {
		t.Fatal("expected GetByID to return false for unknown ID")
	}
}

func TestStore_AddAndList(t *testing.T) {
	s := NewStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	f1 := api.Finding{ID: NewFindingID(), Kind: api.KindStateDivergence, Severity: api.SeverityError, OpenedAt: base.Add(1 * time.Second), Summary: "oldest"}
	f2 := api.Finding{ID: NewFindingID(), Kind: api.KindNodeCrash, Severity: api.SeverityError, OpenedAt: base.Add(2 * time.Second), Summary: "middle"}
	f3 := api.Finding{ID: NewFindingID(), Kind: api.KindChaosViolation, Severity: api.SeverityError, OpenedAt: base.Add(3 * time.Second), Summary: "newest"}

	s.Add(f1)
	s.Add(f2)
	s.Add(f3)

	if s.Len() != 3 {
		t.Fatalf("expected Len 3, got %d", s.Len())
	}

	// newest-first ordering
	list := s.List(ListOpts{})
	if list[0].ID != f3.ID {
		t.Errorf("expected newest first, got %q", list[0].ID)
	}
	if list[2].ID != f1.ID {
		t.Errorf("expected oldest last, got %q", list[2].ID)
	}

	// Kind filter
	crashes := s.List(ListOpts{Kind: api.KindNodeCrash})
	if len(crashes) != 1 || crashes[0].ID != f2.ID {
		t.Errorf("kind filter failed: got %v", crashes)
	}

	// Since filter: results newer than f2's OpenedAt (f2 excluded, only f3 returned)
	sinceList := s.List(ListOpts{Since: f2.ID})
	if len(sinceList) != 1 || sinceList[0].ID != f3.ID {
		t.Errorf("since filter failed: got %v", sinceList)
	}

	// GetByID
	got, ok := s.GetByID(f1.ID)
	if !ok || got.ID != f1.ID {
		t.Errorf("GetByID failed: ok=%v, got=%v", ok, got)
	}
}

func TestStore_Capacity(t *testing.T) {
	s := NewStore()
	findings := makeFindings(1001)

	oldestID := findings[0].ID
	for _, f := range findings {
		s.Add(f)
	}

	if s.Len() != 1000 {
		t.Fatalf("expected Len 1000 after 1001 adds, got %d", s.Len())
	}

	_, ok := s.GetByID(oldestID)
	if ok {
		t.Error("expected oldest finding to have been evicted")
	}

	// The 1001st (index 1000) should still be present.
	_, ok = s.GetByID(findings[1000].ID)
	if !ok {
		t.Error("expected newest finding to be present")
	}
}

func TestStore_ListLimit(t *testing.T) {
	s := NewStore()
	for _, f := range makeFindings(200) {
		s.Add(f)
	}
	list := s.List(ListOpts{Limit: 10})
	if len(list) != 10 {
		t.Fatalf("expected 10, got %d", len(list))
	}
	// Default limit 100
	list = s.List(ListOpts{})
	if len(list) != 100 {
		t.Fatalf("expected default limit 100, got %d", len(list))
	}
}
