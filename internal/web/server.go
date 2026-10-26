package web

import (
	"errors"
	"net/http"
	"time"
)

// Config configures a Server. Salt makes the stored address hashes useless
// outside this server; it must be secret and stable.
type Config struct {
	DBPath       string
	Salt         string
	TrustedProxy string           // only this peer may set X-Forwarded-For
	Now          func() time.Time // nil: time.Now
}

// Server serves the landing page, its assets and the comments API.
type Server struct {
	cfg     Config
	store   *Store
	limiter *Limiter
	mux     *http.ServeMux
}

func NewServer(cfg Config) (*Server, error) {
	if cfg.Salt == "" {
		return nil, errors.New("web: empty salt")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	store, err := OpenStore(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	store.now = cfg.Now
	s := &Server{
		cfg:     cfg,
		store:   store,
		limiter: NewLimiter(3, 10*time.Minute, 60, time.Hour, cfg.Now),
		mux:     http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /api/comments", s.listComments)
	s.mux.HandleFunc("POST /api/comments", s.postComment)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok\n"))
	})
	if err := s.routePages(); err != nil {
		store.Close()
		return nil, err
	}
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) Close() error { return s.store.Close() }
