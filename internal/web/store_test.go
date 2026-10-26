package web

import (
	"errors"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "comments.jsonl")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func ids(cs []Comment) []int64 {
	out := []int64{}
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}

func mustAdd(t *testing.T, s *Store, body string) Comment {
	t.Helper()
	c, err := s.Add(Comment{Body: body, Lang: "uz"}, "iphash")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestStoreAddList(t *testing.T) {
	s, _ := openTemp(t)
	for _, b := range []string{"a", "b", "c"} {
		mustAdd(t, s, b)
	}
	got, err := s.List(0, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if g := ids(got); len(g) != 2 || g[0] != 3 || g[1] != 2 {
		t.Fatalf("List(0,2) ids = %v, want [3 2]", g)
	}
	if got[0].Body != "c" || got[0].CreatedAt.IsZero() {
		t.Fatalf("newest comment = %+v", got[0])
	}
	got, _ = s.List(2, 10, false)
	if g := ids(got); len(g) != 1 || g[0] != 1 {
		t.Fatalf("List(2,10) ids = %v, want [1]", g)
	}
}

func TestStoreHideVisibleAcrossProcesses(t *testing.T) {
	a, path := openTemp(t)
	b, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	mustAdd(t, a, "one")
	mustAdd(t, a, "two")
	if err := b.SetHidden(1, true); err != nil {
		t.Fatal(err)
	}
	got, _ := a.List(0, 10, false)
	if g := ids(got); len(g) != 1 || g[0] != 2 {
		t.Fatalf("visible ids = %v, want [2]", g)
	}
	got, _ = a.List(0, 10, true)
	if g := ids(got); len(g) != 2 {
		t.Fatalf("withHidden ids = %v, want both", g)
	}
	if !got[1].Hidden {
		t.Fatal("comment 1 should report Hidden")
	}
}

func TestStoreReopen(t *testing.T) {
	s, path := openTemp(t)
	mustAdd(t, s, "one")
	if err := s.SetHidden(1, true); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if c := mustAdd(t, s2, "two"); c.ID != 2 {
		t.Fatalf("id after reopen = %d, want 2", c.ID)
	}
	got, _ := s2.List(0, 10, false)
	if g := ids(got); len(g) != 1 || g[0] != 2 {
		t.Fatalf("visible ids = %v, want [2]", g)
	}
}

func TestStoreSetHiddenUnknown(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.SetHidden(42, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
