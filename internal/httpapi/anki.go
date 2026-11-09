package httpapi

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
)

func (s *Server) ankiRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/anki/export", s.ankiExport)
}

// ankiExport builds the package in memory first, so a failure is still a JSON error and never a truncated download.
func (s *Server) ankiExport(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	name, err := s.svc.AnkiExport(r.Context(), r.URL.Query().Get("scope"), &buf)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	buf.WriteTo(w)
}
