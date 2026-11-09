package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/gitx"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/review"
)

// Op is one restructure of the map (ADR-0014). Ids are the usual paths: theme/sub-theme/subject.
//
//   - move: ID is a subject or a sub-theme, To its new id. Moving a subject into another
//     sub-theme and renaming it are the same operation; To may also name an existing
//     sub-theme, then the subject keeps its file name.
//   - split: ID is a subject; each part takes some of its notions to a new subject.
//   - merge: Sources are folded into Into, which is one of them or a new subject id.
type Op struct {
	Kind    string      `json:"kind"`
	ID      string      `json:"id,omitempty"`
	To      string      `json:"to,omitempty"`
	Name    string      `json:"name,omitempty"`
	Parts   []SplitPart `json:"parts,omitempty"`
	Sources []string    `json:"sources,omitempty"`
	Into    string      `json:"into,omitempty"`
}

type SplitPart struct {
	ID      string   `json:"id"`
	Name    string   `json:"name,omitempty"`
	Notions []string `json:"notions"`
}

// FileOp is one step of a plan. Content is the whole new file for moves and writes.
type FileOp struct {
	Kind    string `json:"kind"` // move, write, delete
	Path    string `json:"path"`
	From    string `json:"from,omitempty"`
	Content string `json:"-"`
}

type Plan struct {
	Op      Op              `json:"op"`
	Files   []FileOp        `json:"files"`
	Renames []review.Rename `json:"renames"`
	Summary []string        `json:"summary"`
	Message string          `json:"message"`
	Branch  string          `json:"branch,omitempty"`
	// Hash identifies the file changes; the app sends it back so it applies the plan it showed.
	Hash string `json:"hash"`
}

// liveIDs answers which ids exist in u: themes, sub-themes, subjects and cards. A rename never
// redirects a live id (ADR-0014).
func liveIDs(u *kb.Universe) review.Live {
	ids := map[string]bool{}
	for _, g := range u.Galaxies {
		ids[g.ID] = true
		for _, c := range g.Clusters {
			ids[c.ID] = true
			for _, st := range c.Stars {
				ids[st.ID] = true
				for _, card := range st.Cards {
					ids[card.ID] = true
				}
			}
		}
	}
	return func(id string) bool { return ids[id] }
}

var segment = regexp.MustCompile(`^[\p{Ll}\p{N}][\p{Ll}\p{N}-]*$`)

func validID(id string, parts int) bool {
	segs := strings.Split(id, "/")
	if len(segs) != parts {
		return false
	}
	for _, s := range segs {
		if !segment.MatchString(s) {
			return false
		}
	}
	return true
}

// planner holds the files a plan touches, keyed by their path today, so several steps (a move,
// then a link rewrite in the moved file) end up as one write.
type planner struct {
	s       *Service
	u       *kb.Universe
	now     time.Time
	files   map[string]*FileOp // by current relative path
	order   []string
	renames []review.Rename
	summary []string
	// notions maps "subject#notion" references to where a split or a merge sends the notion.
	notions map[string]string
}

func (s *Service) newPlanner(u *kb.Universe) *planner {
	return &planner{s: s, u: u, now: s.now(), files: map[string]*FileOp{}, notions: map[string]string{}}
}

func (p *planner) rel(abs string) string {
	r, _ := filepath.Rel(p.s.root, abs)
	return filepath.ToSlash(r)
}

func starRel(id string) string { return kb.GalaxiesDir + "/" + id + ".md" }

// content returns what a file will contain after the plan so far.
func (p *planner) content(rel string) (string, error) {
	if f, ok := p.files[rel]; ok {
		if f.Kind == "delete" {
			return "", fmt.Errorf("%s is deleted by this plan: %w", rel, ErrInvalid)
		}
		return f.Content, nil
	}
	b, err := os.ReadFile(filepath.Join(p.s.root, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("restructure: %w", err)
	}
	return string(b), nil
}

func (p *planner) set(rel string, op FileOp) {
	if _, ok := p.files[rel]; !ok {
		p.order = append(p.order, rel)
	}
	p.files[rel] = &op
}

func (p *planner) move(fromRel, toRel string) error {
	c, err := p.content(fromRel)
	if err != nil {
		return err
	}
	p.set(fromRel, FileOp{Kind: "move", From: fromRel, Path: toRel, Content: c})
	return nil
}

func (p *planner) write(rel, content string) {
	if f, ok := p.files[rel]; ok && f.Kind == "move" {
		f.Content = content
		return
	}
	if f, ok := p.files[rel]; ok && f.Kind == "write" {
		f.Content = content
		return
	}
	p.set(rel, FileOp{Kind: "write", Path: rel, Content: content})
}

func (p *planner) rename(from, to string) {
	p.renames = append(p.renames, review.Rename{From: from, To: to, At: p.now})
}

func (p *planner) taken(id string) bool {
	if p.u.Star(id) != nil || p.u.Cluster(id) != nil || p.u.Galaxy(id) != nil {
		return true
	}
	for _, f := range p.files {
		if f.Path == starRel(id) {
			return true
		}
	}
	_, err := os.Stat(filepath.Join(p.s.root, filepath.FromSlash(kb.GalaxiesDir+"/"+id+".md")))
	if err == nil {
		return true
	}
	_, err = os.Stat(filepath.Join(p.s.root, filepath.FromSlash(kb.GalaxiesDir+"/"+id)))
	return err == nil
}

type rawMeta struct {
	Name     string   `yaml:"name"`
	Level    int      `yaml:"level"`
	State    kb.State `yaml:"state"`
	Links    []string `yaml:"links"`
	Evidence []string `yaml:"evidence"`
	Tags     []string `yaml:"tags"`
}

func parseFile(content string) (rawMeta, string, error) {
	front, body, err := kb.SplitFile([]byte(content))
	if err != nil {
		return rawMeta{}, "", err
	}
	var m rawMeta
	if len(front) > 0 {
		if err := yaml.Unmarshal(front, &m); err != nil {
			return rawMeta{}, "", err
		}
	}
	return m, string(body), nil
}

func setFields(content string, fields map[string]any) (string, error) {
	b, err := kb.SetFieldsBytes([]byte(content), fields)
	return string(b), err
}

// replaceBody keeps the frontmatter bytes exactly and swaps the body.
func replaceBody(content, body string) string {
	if !strings.HasPrefix(content, "---") {
		return body
	}
	front, _, err := kb.SplitFile([]byte(content))
	if err != nil {
		return content
	}
	return "---\n" + string(front) + "---\n" + body
}

// ErrPlanChanged means the files changed between the plan the user saw and the apply.
var ErrPlanChanged = errors.New("the plan changed since it was shown")

// Restructure plans an operation and, unless dryRun, applies it on the user's branch with one
// commit: the user asked for it from the app or the CLI. A non-empty expect is the hash of the
// plan the user saw; a different plan today is refused instead of applied.
func (s *Service) Restructure(ctx context.Context, op Op, dryRun bool, expect string) (Plan, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	plan, err := s.plan(op)
	if err != nil || dryRun {
		return plan, err
	}
	if expect != "" && expect != plan.Hash {
		return plan, fmt.Errorf("%w: look at the new plan before applying: %w", ErrPlanChanged, ErrInvalid)
	}
	if err := s.applyPlan(ctx, plan); err != nil {
		return plan, err
	}
	return plan, s.Reload(ctx)
}

// ProposeRestructure plans an operation and writes it on proposals/<slug> for the user to review,
// the agent path of ADR-0004. Each restructure gets its own branch: an existing one is refused,
// so an earlier proposal the user has not merged yet is never overwritten.
func (s *Service) ProposeRestructure(ctx context.Context, op Op, slug string, dryRun bool) (Plan, error) {
	s.writeMu.Lock()
	plan, err := s.plan(op)
	s.writeMu.Unlock()
	if err != nil || dryRun {
		return plan, err
	}
	if s.git == nil {
		return plan, fmt.Errorf("restructure proposals need the knowledge folder to be a git repository: %w", ErrInvalid)
	}
	if slug == "" {
		slug = "restructure-" + kb.Slug(op.Kind+"-"+firstNonEmpty(op.ID, op.Into)) + "-" + plan.Hash[:7]
	}
	if s.git.HasBranch(ctx, "proposals/"+slug) {
		return plan, fmt.Errorf("proposals/%s already exists and may hold work not merged yet; pick another slug or leave it empty for a unique one: %w", slug, ErrInvalid)
	}
	var files []gitx.FileChange
	for _, f := range plan.Files {
		switch f.Kind {
		case "move":
			files = append(files, gitx.FileChange{Path: f.Path, From: f.From, Content: f.Content, Restructure: true})
		case "write":
			files = append(files, gitx.FileChange{Path: f.Path, Content: f.Content, Restructure: true})
		case "delete":
			files = append(files, gitx.FileChange{Path: f.Path, Delete: true, Restructure: true})
		}
	}
	if len(plan.Renames) > 0 {
		lines, err := review.RenameLines(plan.Renames)
		if err != nil {
			return plan, err
		}
		old, err := os.ReadFile(s.book.RenamesFile())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return plan, fmt.Errorf("restructure: %w", err)
		}
		files = append(files, gitx.FileChange{Path: gitx.RenamesFile, Content: string(old) + string(lines), Restructure: true})
	}
	branch, err := s.Propose(ctx, slug, plan.Message, files)
	plan.Branch = branch
	return plan, err
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

func (s *Service) plan(op Op) (Plan, error) {
	u, _, _, _ := s.snapshot()
	p := s.newPlanner(u)
	var msg string
	var err error
	switch op.Kind {
	case "move", "rename":
		op.Kind = "move"
		msg, err = p.planMove(op)
	case "split":
		msg, err = p.planSplit(op)
	case "merge":
		msg, err = p.planMerge(op)
	default:
		err = fmt.Errorf("unknown restructure %q, use move, split or merge: %w", op.Kind, ErrInvalid)
	}
	if err != nil {
		return Plan{}, err
	}
	if err := p.rewriteLinks(); err != nil {
		return Plan{}, err
	}
	if err := p.keepEmptiedSubThemes(); err != nil {
		return Plan{}, err
	}
	plan := Plan{Op: op, Renames: p.renames, Summary: p.summary, Message: msg, Files: []FileOp{}}
	if plan.Renames == nil {
		plan.Renames = []review.Rename{}
	}
	for _, rel := range p.order {
		plan.Files = append(plan.Files, *p.files[rel])
	}
	plan.Hash = plan.hash()
	return plan, nil
}

// hash identifies what a plan would do to the files, without the timestamps, so the plan shown
// in the app can be checked against the one about to be applied.
func (p Plan) hash() string {
	h := sha256.New()
	for _, f := range p.Files {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%d\x00%s\x00", f.Kind, f.From, f.Path, len(f.Content), f.Content)
	}
	for _, r := range p.Renames {
		fmt.Fprintf(h, "%s\x00%s\x00", r.From, r.To)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// keepEmptiedSubThemes writes a _cluster.md for a sub-theme this plan leaves without subjects:
// git does not keep empty folders, so the sub-theme would vanish from every other clone.
func (p *planner) keepEmptiedSubThemes() error {
	for _, g := range p.u.Galaxies {
		for _, cl := range g.Clusters {
			if len(cl.Stars) == 0 {
				continue
			}
			clusterRel := p.rel(cl.Path)
			if f, ok := p.files[clusterRel]; ok && f.Kind == "move" {
				continue // the sub-theme itself moves
			}
			dir := kb.GalaxiesDir + "/" + cl.ID + "/"
			left := map[string]bool{}
			for _, st := range cl.Stars {
				rel := p.rel(st.Path)
				if f, ok := p.files[rel]; !ok || f.Kind == "write" {
					left[rel] = true
				}
			}
			for _, f := range p.files {
				name := strings.TrimPrefix(f.Path, dir)
				if f.Kind != "delete" && name != f.Path && !strings.Contains(name, "/") && !strings.HasPrefix(name, "_") {
					left[f.Path] = true
				}
			}
			if len(left) > 0 {
				continue
			}
			if _, err := os.Stat(cl.Path); err == nil {
				continue // its _cluster.md stays and keeps the folder
			}
			p.write(clusterRel, "---\nname: "+strconv.Quote(cl.Name)+"\n---\n")
			p.summary = append(p.summary, fmt.Sprintf("%s garde son dossier (_cluster.md créé)", cl.ID))
		}
	}
	return nil
}

// --- move ---

func (p *planner) planMove(op Op) (string, error) {
	if st := p.u.Star(op.ID); st != nil {
		return p.moveStar(st, op.To, op.Name)
	}
	if c := p.u.Cluster(op.ID); c != nil {
		return p.moveCluster(c, op.To, op.Name)
	}
	return "", fmt.Errorf("%q is neither a subject nor a sub-theme: %w", op.ID, ErrNotFound)
}

func (p *planner) moveStar(st *kb.Star, to, name string) (string, error) {
	if to == "" {
		to = st.ID
	}
	if p.u.Cluster(to) != nil {
		to += "/" + st.ID[strings.LastIndex(st.ID, "/")+1:]
	}
	if !validID(to, 3) {
		return "", fmt.Errorf("target %q must be theme/sub-theme/subject in lowercase words joined by dashes: %w", to, ErrInvalid)
	}
	cluster := to[:strings.LastIndex(to, "/")]
	if p.u.Cluster(cluster) == nil {
		return "", fmt.Errorf("sub-theme %q does not exist; create its folder first: %w", cluster, ErrInvalid)
	}
	if to == st.ID && (name == "" || name == st.Name) {
		return "", fmt.Errorf("nothing to change for %q: %w", st.ID, ErrInvalid)
	}
	from := p.rel(st.Path)
	if to != st.ID {
		if p.taken(to) {
			return "", fmt.Errorf("%q already exists: %w", to, ErrInvalid)
		}
		if err := p.move(from, starRel(to)); err != nil {
			return "", err
		}
		p.rename(st.ID, to)
		p.summary = append(p.summary, fmt.Sprintf("%s → %s", st.ID, to))
	}
	if name != "" && name != st.Name {
		c, err := p.content(from)
		if err != nil {
			return "", err
		}
		if c, err = setFields(c, map[string]any{"name": name}); err != nil {
			return "", fmt.Errorf("restructure: %w", err)
		}
		p.write(from, c)
		p.summary = append(p.summary, fmt.Sprintf("nom : %s → %s", st.Name, name))
	}
	label := firstNonEmpty(name, st.Name)
	if to == st.ID {
		return fmt.Sprintf("refactor(knowledge): rename %s to %s", st.Name, label), nil
	}
	return fmt.Sprintf("refactor(knowledge): move %s to %s", label, to), nil
}

func (p *planner) moveCluster(c *kb.Cluster, to, name string) (string, error) {
	if to == "" {
		to = c.ID
	}
	if p.u.Galaxy(to) != nil {
		to += "/" + c.ID[strings.LastIndex(c.ID, "/")+1:]
	}
	if !validID(to, 2) {
		return "", fmt.Errorf("target %q must be theme/sub-theme in lowercase words joined by dashes: %w", to, ErrInvalid)
	}
	if p.u.Galaxy(to[:strings.Index(to, "/")]) == nil {
		return "", fmt.Errorf("theme %q does not exist: %w", to[:strings.Index(to, "/")], ErrInvalid)
	}
	if to == c.ID && (name == "" || name == c.Name) {
		return "", fmt.Errorf("nothing to change for %q: %w", c.ID, ErrInvalid)
	}
	if to != c.ID {
		if p.taken(to) {
			return "", fmt.Errorf("%q already exists: %w", to, ErrInvalid)
		}
		for _, st := range c.Stars {
			nid := to + "/" + st.ID[strings.LastIndex(st.ID, "/")+1:]
			if err := p.move(p.rel(st.Path), starRel(nid)); err != nil {
				return "", err
			}
			p.rename(st.ID, nid)
		}
		if _, err := os.Stat(c.Path); err == nil {
			if err := p.move(p.rel(c.Path), kb.GalaxiesDir+"/"+to+"/_cluster.md"); err != nil {
				return "", err
			}
		}
		p.rename(c.ID, to)
		p.summary = append(p.summary, fmt.Sprintf("%s → %s (%d sujets)", c.ID, to, len(c.Stars)))
	}
	if name != "" && name != c.Name {
		rel := p.rel(c.Path)
		content := "---\nname: " + name + "\n---\n"
		if _, err := os.Stat(c.Path); err == nil {
			var err error
			if content, err = p.content(rel); err != nil {
				return "", err
			}
			if content, err = setFields(content, map[string]any{"name": name}); err != nil {
				return "", fmt.Errorf("restructure: %w", err)
			}
		}
		if to != c.ID {
			if f, ok := p.files[rel]; ok {
				f.Content = content
			} else {
				p.set(rel, FileOp{Kind: "write", Path: kb.GalaxiesDir + "/" + to + "/_cluster.md", Content: content})
			}
		} else {
			p.write(rel, content)
		}
		p.summary = append(p.summary, fmt.Sprintf("nom : %s → %s", c.Name, name))
	}
	return fmt.Sprintf("refactor(knowledge): move sub-theme %s to %s", firstNonEmpty(name, c.Name), to), nil
}

// --- split ---

func (p *planner) planSplit(op Op) (string, error) {
	st := p.u.Star(op.ID)
	if st == nil {
		return "", fmt.Errorf("subject %q: %w", op.ID, ErrNotFound)
	}
	if len(op.Parts) == 0 {
		return "", fmt.Errorf("split needs at least one part: %w", ErrInvalid)
	}
	rel := p.rel(st.Path)
	content, err := p.content(rel)
	if err != nil {
		return "", err
	}
	meta, body, err := parseFile(content)
	if err != nil {
		return "", fmt.Errorf("restructure %s: %w", rel, err)
	}
	pre, sections := kb.Sections(body)
	owner := make([]int, len(sections))
	for i := range owner {
		owner[i] = -1
	}
	cluster := st.Cluster.ID
	ids := map[string]bool{}
	for pi, part := range op.Parts {
		id := part.ID
		if !strings.Contains(id, "/") {
			id = cluster + "/" + id
		}
		op.Parts[pi].ID = id
		if !validID(id, 3) || p.u.Cluster(id[:strings.LastIndex(id, "/")]) == nil {
			return "", fmt.Errorf("part %q must be a new subject id in an existing sub-theme: %w", part.ID, ErrInvalid)
		}
		if p.taken(id) || ids[id] {
			return "", fmt.Errorf("%q already exists: %w", id, ErrInvalid)
		}
		ids[id] = true
		if len(part.Notions) == 0 {
			return "", fmt.Errorf("part %q takes no notion: %w", id, ErrInvalid)
		}
		for _, n := range part.Notions {
			found := false
			for si, sec := range sections {
				if sec.Name == n || sec.Slug == kb.Slug(n) {
					if owner[si] >= 0 {
						return "", fmt.Errorf("notion %q is given to two parts: %w", n, ErrInvalid)
					}
					owner[si], found = pi, true
					break
				}
			}
			if !found {
				var names []string
				for _, sec := range sections {
					names = append(names, sec.Name)
				}
				return "", fmt.Errorf("notion %q is not in %s; notions: %s: %w", n, st.ID, strings.Join(names, ", "), ErrInvalid)
			}
		}
	}
	keep := pre
	for si, sec := range sections {
		if owner[si] < 0 {
			keep += "\n\n" + strings.TrimRight(sec.Text, "\n")
		}
	}
	p.write(rel, replaceBody(content, strings.TrimLeft(keep, "\n")+"\n"))
	for pi, part := range op.Parts {
		var text []string
		var first kb.Section
		for si, sec := range sections {
			if owner[si] == pi {
				if len(text) == 0 {
					first = sec
				}
				text = append(text, strings.TrimRight(sec.Text, "\n"))
			}
		}
		name := part.Name
		if name == "" {
			name = first.Name
			if len(text) > 1 {
				name = titleFromSlug(part.ID[strings.LastIndex(part.ID, "/")+1:])
			}
		}
		front := map[string]any{"name": name, "level": meta.Level, "state": string(st.State), "links": []string{st.ID}}
		if len(meta.Tags) > 0 {
			front["tags"] = meta.Tags
		}
		fm, err := yaml.Marshal(orderedFront(front))
		if err != nil {
			return "", fmt.Errorf("restructure: %w", err)
		}
		partBody := fmt.Sprintf("%s\n\n%s\n", splitNote(st.Name), strings.Join(text, "\n\n"))
		p.set(starRel(part.ID), FileOp{Kind: "write", Path: starRel(part.ID), Content: "---\n" + string(fm) + "---\n" + partBody})
		for _, card := range kb.CardsIn(st.ID, strings.Join(text, "\n")) {
			p.rename(card.ID, part.ID+card.ID[strings.LastIndexByte(card.ID, ':'):])
		}
		for si, sec := range sections {
			if owner[si] == pi {
				p.notions[st.ID+"#"+sec.Slug] = part.ID + "#" + sec.Slug
			}
		}
		p.summary = append(p.summary, fmt.Sprintf("%s → %s (%d notion%s)", st.ID, part.ID, len(text), plural(len(text))))
	}
	return fmt.Sprintf("refactor(knowledge): split %s into %d", st.Name, len(op.Parts)+1), nil
}

func splitNote(origin string) string { return fmt.Sprintf("Détaché de « %s ».", origin) }

// orderedFront keeps name first so new files read like hand-written ones.
func orderedFront(m map[string]any) yaml.Node {
	var n yaml.Node
	n.Kind = yaml.MappingNode
	keys := []string{"name", "level", "state", "links", "evidence", "tags"}
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		var vn yaml.Node
		_ = vn.Encode(v)
		if vn.Kind == yaml.SequenceNode {
			vn.Style = yaml.FlowStyle
		}
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &vn)
	}
	return n
}

func titleFromSlug(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// --- merge ---

var stateRank = map[kb.State]int{kb.StateIdea: 0, kb.StateExplore: 1, kb.StateProgress: 2, kb.StateAcquired: 3}

func (p *planner) planMerge(op Op) (string, error) {
	if len(op.Sources) < 2 && !(len(op.Sources) == 1 && op.Into != "" && op.Into != op.Sources[0]) {
		return "", fmt.Errorf("merge needs two subjects: %w", ErrInvalid)
	}
	var stars []*kb.Star
	seen := map[string]bool{}
	for _, id := range op.Sources {
		st := p.u.Star(id)
		if st == nil {
			return "", fmt.Errorf("subject %q: %w", id, ErrNotFound)
		}
		if !seen[id] {
			stars = append(stars, st)
			seen[id] = true
		}
	}
	into := op.Into
	if into == "" {
		into = stars[0].ID
	}
	var target *kb.Star
	if st := p.u.Star(into); st != nil {
		target = st
		if !seen[into] {
			stars = append([]*kb.Star{st}, stars...)
			seen[into] = true
		}
	} else {
		if !validID(into, 3) || p.u.Cluster(into[:strings.LastIndex(into, "/")]) == nil {
			return "", fmt.Errorf("target %q must be an existing subject or a new id in an existing sub-theme: %w", into, ErrInvalid)
		}
		if p.taken(into) {
			return "", fmt.Errorf("%q already exists: %w", into, ErrInvalid)
		}
	}
	if len(stars) < 2 {
		return "", fmt.Errorf("merge needs two different subjects: %w", ErrInvalid)
	}
	base := stars[0]
	if target != nil {
		base = target
	}
	baseRel := p.rel(base.Path)
	content, err := p.content(baseRel)
	if err != nil {
		return "", err
	}
	_, body, err := parseFile(content)
	if err != nil {
		return "", fmt.Errorf("restructure %s: %w", baseRel, err)
	}
	level, state := 0, kb.StateIdea
	var links, evidence, tags []string
	known := map[string]string{} // question -> answer already in the merged body
	for _, c := range kb.CardsIn(base.ID, body) {
		known[strings.TrimSpace(c.Question)] = c.Answer
	}
	var parts []string
	for _, st := range stars {
		c, err := p.content(p.rel(st.Path))
		if err != nil {
			return "", err
		}
		m, b, err := parseFile(c)
		if err != nil {
			return "", fmt.Errorf("restructure %s: %w", p.rel(st.Path), err)
		}
		level = max(level, st.Level)
		if stateRank[st.State] > stateRank[state] {
			state = st.State
		}
		links = append(links, m.Links...)
		evidence = append(evidence, m.Evidence...)
		tags = append(tags, m.Tags...)
		if st == base {
			continue
		}
		var keep []string
		inFence := false
		for _, line := range strings.Split(b, "\n") {
			if t := strings.TrimSpace(line); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
				inFence = !inFence
			}
			// A "q :: a" line inside a code block is code, not a card: never deduplicate it.
			var cards []kb.Card
			if !inFence {
				cards = kb.CardsIn(st.ID, line)
			}
			if len(cards) == 1 {
				q := strings.TrimSpace(cards[0].Question)
				if a, ok := known[q]; ok {
					if a == cards[0].Answer {
						continue
					}
					return "", fmt.Errorf("question %q has two answers (%q in %s, %q in the merge): edit one first: %w", q, cards[0].Answer, st.ID, a, ErrInvalid)
				}
				known[q] = cards[0].Answer
			}
			keep = append(keep, line)
		}
		rest := strings.Join(keep, "\n")
		// A part split out of the base comes back as the notions it was, not under its own heading.
		pre, secs := kb.Sections(rest)
		if strings.TrimSpace(pre) == splitNote(base.Name) && len(secs) > 0 {
			for _, sec := range secs {
				parts = append(parts, strings.TrimRight(sec.Text, "\n"))
				p.notions[st.ID+"#"+sec.Slug] = into + "#" + sec.Slug
			}
			continue
		}
		parts = append(parts, fmt.Sprintf("## %s\n\n%s", st.Name, strings.TrimSpace(demote(rest))))
		// Its notions become sub-headings: a link to one of them now lands on the source's section.
		for _, sec := range secs {
			p.notions[st.ID+"#"+sec.Slug] = into + "#" + kb.Slug(st.Name)
		}
	}
	drop := map[string]bool{into: true}
	for _, st := range stars {
		drop[st.ID] = true
		drop[st.ID[strings.LastIndex(st.ID, "/")+1:]] = true
	}
	fields := map[string]any{"level": level, "state": string(state)}
	if l := uniq(links, drop); len(l) > 0 {
		fields["links"] = l
	}
	if e := uniq(evidence, nil); len(e) > 0 {
		fields["evidence"] = e
	}
	if t := uniq(tags, nil); len(t) > 0 {
		fields["tags"] = t
	}
	if op.Name != "" {
		fields["name"] = op.Name
	}
	merged, err := setFields(content, fields)
	if err != nil {
		return "", fmt.Errorf("restructure: %w", err)
	}
	merged = replaceBody(merged, strings.TrimRight(body, "\n")+"\n\n"+strings.Join(parts, "\n\n")+"\n")
	if target == nil {
		if err := p.move(baseRel, starRel(into)); err != nil {
			return "", err
		}
		p.rename(base.ID, into)
	}
	p.write(baseRel, merged)
	names := []string{}
	for _, st := range stars {
		names = append(names, st.Name)
		if st == base {
			continue
		}
		p.set(p.rel(st.Path), FileOp{Kind: "delete", Path: p.rel(st.Path)})
		p.rename(st.ID, into)
	}
	p.summary = append(p.summary, fmt.Sprintf("%s → %s (niveau %d)", strings.Join(op.Sources, " + "), into, level))
	return fmt.Sprintf("refactor(knowledge): merge %s into %s", strings.Join(names, ", "), into), nil
}

// demote turns the merged subject's own notions into sub-headings under its name.
func demote(body string) string {
	lines := strings.Split(body, "\n")
	inFence := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") || strings.HasPrefix(strings.TrimSpace(l), "~~~") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(l, "## ") {
			lines[i] = "#" + l
		}
	}
	return strings.Join(lines, "\n")
}

func uniq(xs []string, drop map[string]bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, x := range xs {
		if x == "" || seen[x] || drop[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	return out
}

// --- links ---

// deepLink finds the app's links to a subject or a notion written in a fiche: (#/theme/sub/subject#notion).
var deepLink = regexp.MustCompile(`\(#/([^()\s]+)\)`)

// rewriteLinks points every reference this plan breaks at its new id, in every subject file the
// plan leaves in place, moved files included: `links:` entries (full ids, bare slugs, id#notion)
// and deep links in the text.
func (p *planner) rewriteLinks() error {
	mapping := map[string]string{}
	for _, r := range p.renames {
		if !strings.Contains(r.From, ":") {
			mapping[r.From] = r.To
		}
	}
	if len(mapping) == 0 && len(p.notions) == 0 {
		return nil
	}
	slugs := map[string][]string{}
	for _, st := range p.u.Stars() {
		slug := st.ID[strings.LastIndex(st.ID, "/")+1:]
		slugs[slug] = append(slugs[slug], st.ID)
	}
	// resolve maps one reference; bareSlug allows the short form `links:` accepts.
	resolve := func(ref string, bareSlug bool) (string, bool) {
		if to, ok := p.notions[ref]; ok {
			return to, true
		}
		base, frag, hasFrag := strings.Cut(ref, "#")
		to, ok := mapping[base]
		if !ok && bareSlug {
			if ids := slugs[base]; len(ids) == 1 {
				if to2, ok2 := p.notions[ids[0]+"#"+frag]; ok2 && hasFrag {
					return to2, true
				}
				to, ok = mapping[ids[0]]
			}
		}
		if !ok {
			return "", false
		}
		if hasFrag {
			return to + "#" + frag, true
		}
		return to, true
	}
	stars := p.u.Stars()
	sort.Slice(stars, func(i, j int) bool { return stars[i].ID < stars[j].ID })
	for _, st := range stars {
		rel := p.rel(st.Path)
		if f, ok := p.files[rel]; ok && f.Kind == "delete" {
			continue
		}
		c, err := p.content(rel)
		if err != nil {
			return err
		}
		m, body, err := parseFile(c)
		if err != nil {
			continue
		}
		self := st.ID
		if f, ok := p.files[rel]; ok && f.Kind == "move" {
			self = strings.TrimSuffix(strings.TrimPrefix(f.Path, kb.GalaxiesDir+"/"), ".md")
		}
		changed := false
		var out []string
		for _, l := range m.Links {
			if to, ok := resolve(l, true); ok {
				l, changed = to, true
			}
			if l == self {
				changed = true
				continue
			}
			if !slices.Contains(out, l) {
				out = append(out, l)
			}
		}
		if changed {
			fields := map[string]any{"links": out}
			if len(out) == 0 {
				fields["links"] = []string{}
			}
			if c, err = setFields(c, fields); err != nil {
				return fmt.Errorf("restructure %s: %w", rel, err)
			}
		}
		newBody := deepLink.ReplaceAllStringFunc(body, func(s string) string {
			if to, ok := resolve(s[3:len(s)-1], false); ok {
				return "(#/" + to + ")"
			}
			return s
		})
		if newBody != body {
			c, changed = replaceBody(c, newBody), true
		}
		if changed {
			p.write(rel, c)
		}
	}
	return nil
}

// --- apply ---

// applyPlan changes the files step by step and records how to undo each step. If any step, the
// rename log or the commit fails, the steps are undone in reverse: the tree, the rename log and
// the index are left as they were, and the error says so.
func (s *Service) applyPlan(ctx context.Context, plan Plan) (err error) {
	var touched, emptied []string
	var undo []func() error
	abs := func(rel string) string { return filepath.Join(s.root, filepath.FromSlash(rel)) }
	defer func() {
		if err == nil {
			return
		}
		var failed []string
		for i := len(undo) - 1; i >= 0; i-- {
			if uerr := undo[i](); uerr != nil {
				failed = append(failed, uerr.Error())
			}
		}
		if len(failed) > 0 {
			s.log.Error("restructure rollback incomplete", "err", strings.Join(failed, "; "))
			err = fmt.Errorf("%w; rollback incomplete, check the files: %s", err, strings.Join(failed, "; "))
		}
	}()
	// keep restores a path to what it holds now: its bytes, or nothing.
	keep := func(path string) error {
		data, rerr := os.ReadFile(path)
		existed := rerr == nil
		if rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			return fmt.Errorf("restructure: %w", rerr)
		}
		undo = append(undo, func() error {
			if !existed {
				if rerr := os.Remove(path); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
					return rerr
				}
				return nil
			}
			if merr := os.MkdirAll(filepath.Dir(path), 0o755); merr != nil {
				return merr
			}
			return kb.WriteFile(path, data)
		})
		return nil
	}
	// mkdir creates a folder and, on rollback, removes the folders it created (if still empty).
	mkdir := func(dir string) error {
		var created []string
		for d := dir; ; d = filepath.Dir(d) {
			if _, serr := os.Stat(d); serr == nil || d == s.root || d == filepath.Dir(d) {
				break
			}
			created = append(created, d)
		}
		if merr := os.MkdirAll(dir, 0o755); merr != nil {
			return fmt.Errorf("restructure: %w", merr)
		}
		undo = append(undo, func() error {
			for _, d := range created {
				os.Remove(d)
			}
			return nil
		})
		return nil
	}
	for i, f := range plan.Files {
		if s.applyHook != nil {
			if err = s.applyHook(i); err != nil {
				return err
			}
		}
		switch f.Kind {
		case "move":
			if err = keep(abs(f.From)); err != nil {
				return err
			}
			if err = keep(abs(f.Path)); err != nil {
				return err
			}
			if err = mkdir(filepath.Dir(abs(f.Path))); err != nil {
				return err
			}
			if err = os.Rename(abs(f.From), abs(f.Path)); err != nil {
				return fmt.Errorf("restructure: move %s: %w", f.From, err)
			}
			if err = kb.WriteFile(abs(f.Path), []byte(f.Content)); err != nil {
				return err
			}
			touched = append(touched, abs(f.From), abs(f.Path))
			emptied = append(emptied, filepath.Dir(abs(f.From)))
		case "write":
			if err = keep(abs(f.Path)); err != nil {
				return err
			}
			if err = mkdir(filepath.Dir(abs(f.Path))); err != nil {
				return err
			}
			if err = kb.WriteFile(abs(f.Path), []byte(f.Content)); err != nil {
				return err
			}
			touched = append(touched, abs(f.Path))
		case "delete":
			if err = keep(abs(f.Path)); err != nil {
				return err
			}
			if err = os.Remove(abs(f.Path)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("restructure: delete %s: %w", f.Path, err)
			}
			err = nil
			touched = append(touched, abs(f.Path))
		}
	}
	if s.applyHook != nil {
		if err = s.applyHook(len(plan.Files)); err != nil {
			return err
		}
	}
	// The rename log is written last among the files, so a failed step never leaves a rename
	// pointing at a subject that did not move.
	if len(plan.Renames) > 0 {
		// Undo runs in reverse: the log file is restored first, then the book replays it.
		undo = append(undo, func() error { return s.book.Reload() })
		if err = keep(s.book.RenamesFile()); err != nil {
			return err
		}
		if err = s.book.AppendRenames(plan.Renames); err != nil {
			return err
		}
		touched = append(touched, s.book.RenamesFile())
	}
	if s.git != nil {
		if err = s.git.Commit(ctx, plan.Message, touched...); err != nil {
			s.git.Unstage(context.WithoutCancel(ctx), touched...)
			return fmt.Errorf("restructure: commit failed, nothing changed: %w", err)
		}
	}
	for _, d := range emptied {
		os.Remove(d) // only succeeds when a moved sub-theme left its folder empty
	}
	return nil
}

// live maps an id from before a restructure to the id it has today; unknown ids pass through.
func (s *Service) live(u *kb.Universe, id string) string {
	if u.Star(id) != nil || u.Cluster(id) != nil || u.Galaxy(id) != nil {
		return id
	}
	base, frag, hasFrag := strings.Cut(id, "#")
	to, ok := s.book.Redirects().ID(base)
	if !ok {
		return id
	}
	if hasFrag {
		return to + "#" + frag
	}
	return to
}
