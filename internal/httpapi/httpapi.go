// Package httpapi serves the JSON API, the MCP endpoint and the web app on localhost.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

type Server struct {
	svc *app.Service
	log *slog.Logger
	web fs.FS
	mcp *mcp.Server
}

func New(svc *app.Service, log *slog.Logger, web fs.FS, mcpServer *mcp.Server) *Server {
	return &Server{svc: svc, log: log, web: web, mcp: mcpServer}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/universe", s.universe)
	mux.HandleFunc("GET /api/star/{id...}", s.star)
	mux.HandleFunc("PATCH /api/star/{id...}", s.patchStar)
	mux.HandleFunc("GET /api/search", s.search)
	mux.HandleFunc("GET /api/review/due", s.due)
	mux.HandleFunc("POST /api/review", s.review)
	mux.HandleFunc("POST /api/review/flush", s.flush)
	mux.HandleFunc("POST /api/capture", s.capture)
	mux.HandleFunc("GET /api/inbox", s.inbox)
	mux.HandleFunc("GET /api/ai", s.aiPreview)
	mux.HandleFunc("GET /api/events", s.events)
	assets := filepath.Join(s.svc.Root(), "assets")
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(os.DirFS(assets))))
	if s.mcp != nil {
		mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.mcp }, nil))
	}
	if s.web != nil {
		mux.Handle("/", spa(s.web))
	}
	return guard(mux)
}

// guard blocks DNS rebinding and cross-site writes: the API only answers to localhost pages.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "localhost" && host != "127.0.0.1" && host != "::1" && host != "[::1]" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func spa(web fs.FS) http.Handler {
	files := http.FileServerFS(web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if _, err := fs.Stat(web, p); err != nil {
				r.URL.Path = "/"
			}
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, app.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		s.log.Error("request failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("body: %v: %w", err, app.ErrInvalid)
	}
	return nil
}

func (s *Server) universe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.Universe())
}

func (s *Server) star(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.Star(r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) patchStar(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Level *int      `json:"level"`
		State *kb.State `json:"state"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	v, err := s.svc.SetMeta(r.Context(), r.PathValue("id"), in.Level, in.State)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	hits, err := s.svc.Search(r.Context(), r.URL.Query().Get("q"), 20)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hits)
}

func (s *Server) due(w http.ResponseWriter, r *http.Request) {
	var exclude []string
	if ex := r.URL.Query().Get("exclude"); ex != "" {
		exclude = strings.Split(ex, ",")
	}
	cards, err := s.svc.Due(r.URL.Query().Get("scope"), exclude)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cards)
}

func (s *Server) review(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Card   string `json:"card"`
		Rating int    `json:"rating"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.svc.Review(r.Context(), in.Card, in.Rating); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) flush(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Count int `json:"count"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	s.svc.FlushReviews(r.Context(), in.Count)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) capture(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	p, err := s.svc.Capture(r.Context(), in.Text, "app")
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"saved": p})
}

func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	items, err := s.svc.Inbox()
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// aiPreview returns exactly what the MCP tool would answer, with its token cost.
func (s *Server) aiPreview(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id, tool := q.Get("id"), q.Get("tool")
	var call string
	var payload any
	var err error
	switch tool {
	case "profile":
		call, payload = "profile()", s.svc.Profile()
	case "subgraph":
		call = fmt.Sprintf("subgraph(%q, depth=1)", id)
		payload, err = s.svc.Subgraph(id, 1)
	case "node":
		call = fmt.Sprintf("node(%q)", id)
		payload, err = s.svc.Node(id)
	default:
		s.fail(w, fmt.Errorf("tool %q: %w", tool, app.ErrInvalid))
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	b, _ := json.MarshalIndent(payload, "", "  ")
	compact, _ := json.Marshal(payload)
	writeJSON(w, http.StatusOK, map[string]any{"call": call, "payload": string(b), "tokens": tokens(len(compact) + len(call)), "full_tokens": s.fullTokens()})
}

func (s *Server) fullTokens() int {
	n := 0
	for _, g := range s.svc.Universe().Galaxies {
		for _, c := range g.Clusters {
			for _, st := range c.Stars {
				if v, err := s.svc.Star(st.ID); err == nil {
					n += len(v.Body) + len(v.Name) + 80
				}
			}
		}
	}
	return tokens(n)
}

func tokens(chars int) int { return (chars*10 + 35) / 36 }

// events pushes the universe version after each reload so the UI refreshes when files change on disk.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch, cancel := s.svc.Subscribe()
	defer cancel()
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	fmt.Fprint(w, ": ok\n\n")
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case v := <-ch:
			fmt.Fprintf(w, "event: reload\ndata: %d\n\n", v)
			fl.Flush()
		case <-tick.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func Serve(ctx context.Context, addr string, h http.Handler, log *slog.Logger) error {
	srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "url", "http://"+addr)
	select {
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
