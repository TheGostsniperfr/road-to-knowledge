package httpapi

import (
	"net/http"
)

func (s *Server) pathRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/paths", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.svc.Paths())
	})
	mux.HandleFunc("GET /api/paths/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.svc.Path(r.PathValue("id"))
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("PATCH /api/paths/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			At string `json:"at"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, err)
			return
		}
		v, err := s.svc.SetPathAt(r.Context(), r.PathValue("id"), in.At)
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})
}
