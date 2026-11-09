package app

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/gitx"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/review"
)

// Read-only views for agents that run without the user watching: an evening triage, a teach-back
// session, a weekly recap. Each one is small enough to call without thinking about tokens.

type DueBrief struct {
	Card     string `json:"card"`
	Star     string `json:"star"`
	Question string `json:"q"`
	Answer   string `json:"a"`
	New      bool   `json:"new,omitempty"`
}

type DueView struct {
	Scope string     `json:"scope"`
	Total int        `json:"total"`
	New   int        `json:"new"`
	Cards []DueBrief `json:"cards"`
}

// DueFor lists what the user should review under scope, overdue cards before new ones.
func (s *Service) DueFor(scope string, limit int) (DueView, error) {
	if limit <= 0 || limit > 50 {
		limit = 15
	}
	cards, err := s.Due(scope, nil)
	if err != nil {
		return DueView{}, err
	}
	v := DueView{Scope: scope, Total: len(cards), Cards: []DueBrief{}}
	for _, c := range cards {
		if c.New {
			v.New++
		}
		if len(v.Cards) < limit {
			v.Cards = append(v.Cards, DueBrief{Card: c.ID, Star: c.StarID, Question: c.Question, Answer: c.Answer, New: c.New})
		}
	}
	return v, nil
}

type RecentReview struct {
	Star    string `json:"star"`
	Name    string `json:"name"`
	Reviews int    `json:"reviews"`
	Again   int    `json:"again"`
}

type RecentView struct {
	Since   string         `json:"since"`
	Commits []gitx.Commit  `json:"commits"`
	Reviews []RecentReview `json:"reviews"`
	Note    string         `json:"note,omitempty"`
}

// Recent tells an agent what moved in the last days: commits on the knowledge repo and review
// activity per subject, with how often the user failed a card ("again").
func (s *Service) Recent(ctx context.Context, days int) (RecentView, error) {
	if days <= 0 || days > 365 {
		days = 7
	}
	since := s.now().AddDate(0, 0, -days)
	v := RecentView{Since: since.Format("2006-01-02"), Commits: []gitx.Commit{}, Reviews: []RecentReview{}}
	if s.git == nil {
		v.Note = "the knowledge folder is not a git repository: no history"
	} else {
		commits, err := s.git.Log(ctx, since, 40)
		if err != nil {
			return RecentView{}, fmt.Errorf("recent: %w", err)
		}
		v.Commits = commits
	}
	u, _, _, _ := s.snapshot()
	starOf := map[string]*kb.Star{}
	for _, c := range u.Cards() {
		starOf[c.ID] = u.Star(c.StarID)
	}
	per := map[string]*RecentReview{}
	for _, e := range s.book.Since(since) {
		st := starOf[e.Card]
		if st == nil {
			continue
		}
		r := per[st.ID]
		if r == nil {
			r = &RecentReview{Star: st.ID, Name: st.Name}
			per[st.ID] = r
		}
		r.Reviews++
		if e.Rating == review.Again {
			r.Again++
		}
	}
	for _, r := range per {
		v.Reviews = append(v.Reviews, *r)
	}
	sort.Slice(v.Reviews, func(i, j int) bool {
		if v.Reviews[i].Reviews != v.Reviews[j].Reviews {
			return v.Reviews[i].Reviews > v.Reviews[j].Reviews
		}
		return v.Reviews[i].Star < v.Reviews[j].Star
	})
	return v, nil
}

type RiskItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Level    int    `json:"level"`
	Days     *int   `json:"last_review_days"`
	Due      int    `json:"due_cards"`
	Notions  int    `json:"notions"`
	Reason   string `json:"reason"`
	riskRank float64
}

// AtRisk ranks known subjects by how likely the user has let them slip: time since the last review
// against the theme's review_after, plus overdue cards. Never-reviewed subjects come after the
// overdue ones, oldest level first. The level itself is never touched (ADR-0003).
func (s *Service) AtRisk(scope string, limit int) ([]RiskItem, error) {
	if limit <= 0 || limit > 30 {
		limit = 8
	}
	u, _, _, _ := s.snapshot()
	if scope != "" && u.Galaxy(scope) == nil && u.Cluster(scope) == nil {
		return nil, fmt.Errorf("scope %q: %w", scope, ErrNotFound)
	}
	out := []RiskItem{}
	for _, st := range u.Stars() {
		if st.Level < 2 || (scope != "" && !strings.HasPrefix(st.ID, scope+"/")) {
			continue
		}
		sum := s.summary(st)
		it := RiskItem{ID: st.ID, Name: st.Name, Level: st.Level, Days: sum.Days, Due: sum.Due, Notions: len(st.Planets)}
		after := float64(st.Cluster.Galaxy.ReviewAfter)
		switch {
		case sum.Days != nil:
			it.riskRank = float64(*sum.Days)/after + 0.15*float64(sum.Due)
			it.Reason = fmt.Sprintf("pas revu depuis %d j (seuil %d j)", *sum.Days, int(after))
		default:
			it.riskRank = 0.5 + 0.1*float64(st.Level)
			it.Reason = "jamais révisé ici"
		}
		if sum.Due > 0 {
			it.Reason += fmt.Sprintf(", %d carte%s à revoir", sum.Due, plural(sum.Due))
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if math.Abs(out[i].riskRank-out[j].riskRank) > 1e-9 {
			return out[i].riskRank > out[j].riskRank
		}
		return out[i].ID < out[j].ID
	})
	return capList(out, limit), nil
}

// RawFile returns a subject file as it is on disk, so an agent can edit it and send it back
// through propose without guessing the frontmatter.
func (s *Service) RawFile(id string) (string, error) {
	u, _, _, _ := s.snapshot()
	id = s.live(u, id)
	st := u.Star(id)
	if st == nil {
		return "", fmt.Errorf("star %q: %w", id, ErrNotFound)
	}
	b, err := os.ReadFile(st.Path)
	if err != nil {
		return "", fmt.Errorf("raw %s: %w", id, err)
	}
	return string(b), nil
}

type InboxNote struct {
	File     string `json:"file"`
	Captured string `json:"captured,omitempty"`
	Text     string `json:"text"`
}

type InboxView struct {
	Total int         `json:"total"`
	Notes []InboxNote `json:"notes"`
}

const inboxNoteMax = 600

// InboxFor lists captured notes, oldest first since triage goes in order, each cut to a few hundred characters.
func (s *Service) InboxFor(limit int) (InboxView, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	items, err := s.Inbox()
	if err != nil {
		return InboxView{}, err
	}
	v := InboxView{Total: len(items), Notes: []InboxNote{}}
	for i := len(items) - 1; i >= 0 && len(v.Notes) < limit; i-- {
		t := items[i].Text
		if r := []rune(t); len(r) > inboxNoteMax {
			t = string(r[:inboxNoteMax]) + "…"
		}
		v.Notes = append(v.Notes, InboxNote{File: "inbox/" + items[i].Name, Captured: items[i].Captured, Text: t})
	}
	return v, nil
}
