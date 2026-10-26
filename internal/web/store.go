// Package web serves the ochq.uz landing page and its comments API.
package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"slices"
	"sync"
	"time"
)

var ErrNotFound = errors.New("web: comment not found")

// Comment is one visitor comment. An empty Name is shown as "ghost".
type Comment struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Body      string    `json:"body"`
	Lang      string    `json:"lang"`
	CreatedAt time.Time `json:"created_at"`
	Hidden    bool      `json:"-"`
}

// record is one line of the log. Comments are never rewritten: hiding one
// appends a "hide" record, so the moderation CLI can append to the same
// file while the server runs.
type record struct {
	Op        string    `json:"op"` // add, hide, unhide
	ID        int64     `json:"id"`
	Name      string    `json:"name,omitempty"`
	Body      string    `json:"body,omitempty"`
	Lang      string    `json:"lang,omitempty"`
	CreatedAt time.Time `json:"created_at,omitzero"`
	IP        string    `json:"ip,omitempty"` // salted hash, never the address
}

// Store keeps comments in an append-only JSON-lines file and mirrors them
// in memory. Before every operation it reads what other processes appended.
type Store struct {
	mu       sync.Mutex
	f        *os.File
	offset   int64
	comments []Comment // ordered by ID
	byID     map[int64]int
	now      func() time.Time
}

func OpenStore(path string) (*Store, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	s := &Store{f: f, byID: map[int64]int{}, now: time.Now}
	if err := s.sync(); err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.f.Close() }

// sync applies complete lines appended since the last read. A torn last
// line (a writer still busy) is left for the next call. Must hold s.mu.
func (s *Store) sync() error {
	buf, err := io.ReadAll(io.NewSectionReader(s.f, s.offset, 1<<62))
	if err != nil {
		return err
	}
	for {
		i := bytes.IndexByte(buf, '\n')
		if i < 0 {
			return nil
		}
		line := buf[:i]
		buf = buf[i+1:]
		s.offset += int64(i + 1)
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r record
		if err := json.Unmarshal(line, &r); err != nil {
			// One bad line (a write cut short by a full disk or a crash)
			// must not take the whole page down.
			log.Printf("web: skipping corrupt comment log line at byte %d: %v", s.offset-int64(i+1), err)
			continue
		}
		s.apply(r)
	}
}

func (s *Store) apply(r record) {
	switch r.Op {
	case "add":
		s.byID[r.ID] = len(s.comments)
		s.comments = append(s.comments, Comment{ID: r.ID, Name: r.Name, Body: r.Body, Lang: r.Lang, CreatedAt: r.CreatedAt})
	case "hide", "unhide":
		if i, ok := s.byID[r.ID]; ok {
			s.comments[i].Hidden = r.Op == "hide"
		}
	}
}

// write appends r as one line in a single write, so concurrent appenders
// never interleave inside a line. Must hold s.mu.
func (s *Store) write(r record) error {
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	// sync has read every complete line, so bytes past s.offset are a
	// fragment left by a writer that died. End it, or this record would be
	// glued onto it.
	if fi, err := s.f.Stat(); err != nil {
		return err
	} else if fi.Size() > s.offset {
		line = append([]byte{'\n'}, line...)
	}
	if _, err := s.f.Write(append(line, '\n')); err != nil {
		return err
	}
	if err := s.f.Sync(); err != nil {
		return err
	}
	return s.sync()
}

// Add stores c with the next ID and the current time and returns it.
func (s *Store) Add(c Comment, ipHash string) (Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.sync(); err != nil {
		return Comment{}, err
	}
	var id int64 = 1
	if n := len(s.comments); n > 0 {
		id = s.comments[n-1].ID + 1
	}
	r := record{Op: "add", ID: id, Name: c.Name, Body: c.Body, Lang: c.Lang, CreatedAt: s.now().UTC().Truncate(time.Second), IP: ipHash}
	if err := s.write(r); err != nil {
		return Comment{}, err
	}
	return s.comments[s.byID[id]], nil
}

// List returns up to limit comments older than before (all when before <= 0),
// newest first.
func (s *Store) List(before int64, limit int, withHidden bool) ([]Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.sync(); err != nil {
		return nil, err
	}
	out := []Comment{}
	for _, c := range slices.Backward(s.comments) {
		if len(out) >= limit {
			break
		}
		if (before > 0 && c.ID >= before) || (c.Hidden && !withHidden) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *Store) SetHidden(id int64, hidden bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.sync(); err != nil {
		return err
	}
	if _, ok := s.byID[id]; !ok {
		return ErrNotFound
	}
	op := "unhide"
	if hidden {
		op = "hide"
	}
	return s.write(record{Op: op, ID: id})
}
