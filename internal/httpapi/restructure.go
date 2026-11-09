package httpapi

import (
	"errors"
	"net/http"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
)

// restructureRoutes serves the "Réorganiser" menu: the user asked, so the change lands on the
// current branch with one commit. dry_run returns the plan the menu shows before applying.
func (s *Server) restructureRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/restructure", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			app.Op
			DryRun   bool   `json:"dry_run"`
			PlanHash string `json:"plan_hash"`
		}
		if err := decode(r, &in); err != nil {
			s.fail(w, err)
			return
		}
		plan, err := s.svc.Restructure(r.Context(), in.Op, in.DryRun, in.PlanHash)
		if errors.Is(err, app.ErrPlanChanged) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "plan": plan})
			return
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, plan)
	})
}
