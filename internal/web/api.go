package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxNameRunes = 40
	maxBodyRunes = 1000
	maxRequest   = 8 << 10
	pageSize     = 20
	maxPageSize  = 50
)

var commentLangs = map[string]bool{"uz": true, "en": true, "ru": true}

type commentRequest struct {
	Name    string `json:"name"`
	Body    string `json:"body"`
	Lang    string `json:"lang"`
	Website string `json:"website"` // honeypot: hidden from people, bots fill it
}

func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		limit = pageSize
	}
	cs, err := s.store.List(before, min(limit, maxPageSize), false)
	if err != nil {
		log.Printf("list comments: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) postComment(w http.ResponseWriter, r *http.Request) {
	var req commentRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequest)).Decode(&req); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			http.Error(w, "comment too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	c := Comment{Name: strings.TrimSpace(req.Name), Body: strings.TrimSpace(req.Body), Lang: req.Lang}
	if msg := validate(c); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	ip := s.hashIP(ClientIP(r, s.cfg.TrustedProxy))
	if !s.limiter.Allow(ip) {
		http.Error(w, "too many comments, try again later", http.StatusTooManyRequests)
		return
	}
	if req.Website != "" {
		// Look successful so the bot moves on, but keep nothing.
		writeJSON(w, http.StatusCreated, c)
		return
	}
	saved, err := s.store.Add(c, ip)
	if err != nil {
		log.Printf("add comment: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

// validate returns why c cannot be stored, or "".
func validate(c Comment) string {
	switch {
	case !commentLangs[c.Lang]:
		return "unknown language"
	case c.Body == "":
		return "comment is empty"
	case utf8.RuneCountInString(c.Body) > maxBodyRunes:
		return "comment is too long"
	case utf8.RuneCountInString(c.Name) > maxNameRunes:
		return "name is too long"
	case !printable(c.Body, true) || !printable(c.Name, false):
		return "invalid characters"
	}
	return ""
}

// printable rejects control characters and invalid UTF-8; the body may
// keep newlines and tabs.
func printable(s string, multiline bool) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if (r < 0x20 || r == 0x7f) && !(multiline && (r == '\n' || r == '\t')) {
			return false
		}
	}
	return true
}

func (s *Server) hashIP(ip string) string {
	sum := sha256.Sum256([]byte(s.cfg.Salt + ip))
	return hex.EncodeToString(sum[:8])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
