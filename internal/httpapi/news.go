package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
)

func (s *Server) newsList(w http.ResponseWriter, r *http.Request) {
	q, err := newsQuery(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	v, err := s.svc.News(q)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) newsFetch(w http.ResponseWriter, r *http.Request) {
	rep, err := s.svc.FetchNews(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func newsQuery(r *http.Request) (app.NewsQuery, error) {
	q := app.NewsQuery{Scope: r.URL.Query().Get("scope"), All: r.URL.Query().Get("all") == "1"}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return q, fmt.Errorf("limit %q: %w", v, app.ErrInvalid)
		}
		q.Limit = n
	}
	if v := r.URL.Query().Get("since"); v != "" {
		t, err := app.ParseSince(v, time.Now())
		if err != nil {
			return q, err
		}
		q.Since = t
	}
	return q, nil
}
