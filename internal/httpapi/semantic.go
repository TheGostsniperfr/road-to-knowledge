package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
)

func (s *Server) semanticRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/semantic", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.svc.SemanticStatus())
	})
	mux.HandleFunc("GET /api/neighbors/{id...}", func(w http.ResponseWriter, r *http.Request) {
		limit := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			n, err := strconv.Atoi(l)
			if err != nil || n < 1 {
				s.fail(w, fmt.Errorf("limit %q: %w", l, app.ErrInvalid))
				return
			}
			limit = n
		}
		n, err := s.svc.Neighbors(r.PathValue("id"), limit)
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, n)
	})
}
