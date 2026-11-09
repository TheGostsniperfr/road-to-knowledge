package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
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
}

func (s *Service) newPlanner(u *kb.Universe) *planner {
	return &planner{s: s, u: u, now: s.now(), files: map[string]*FileOp{}}
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

// Restructure plans an operation and, unless dryRun, applies it on the user's branch with one
// commit: the user asked for it from the app or the CLI.
func (s *Service) Restructure(ctx context.Context, op Op, dryRun bool) (Plan, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	plan, err := s.plan(op)
	if err != nil || dryRun {
		return plan, err
	}
	if err := s.applyPlan(ctx, plan); err != nil {
		return plan, err
	}
	return plan, s.Reload(ctx)
}

// ProposeRestructure plans an operation and writes it on proposals/<slug> for the user to review,
// the agent path of ADR-0004.
func (s *Service) ProposeRestructure(ctx context.Context, op Op, slug string, dryRun bool) (Plan, error) {
	s.writeMu.Lock()
	plan, err := s.plan(op)
	s.writeMu.Unlock()
	if err != nil || dryRun {
		return plan, err
	}
	if slug == "" {
		slug = "restructure-" + kb.Slug(op.Kind+"-"+firstNonEmpty(op.ID, op.Into))
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
	plan := Plan{Op: op, Renames: p.renames, Summary: p.summary, Message: msg, Files: []FileOp{}}
	if plan.Renames == nil {
		plan.Renames = []review.Rename{}
	}
	for _, rel := range p.order {
		plan.Files = append(plan.Files, *p.files[rel])
	}
	return plan, nil
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
		for _, line := range strings.Split(b, "\n") {
			cards := kb.CardsIn(st.ID, line)
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
		if pre, secs := kb.Sections(rest); strings.TrimSpace(pre) == splitNote(base.Name) && len(secs) > 0 {
			for _, sec := range secs {
				parts = append(parts, strings.TrimRight(sec.Text, "\n"))
			}
			continue
		}
		parts = append(parts, fmt.Sprintf("## %s\n\n%s", st.Name, strings.TrimSpace(demote(rest))))
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

// rewriteLinks points every link at an id this plan renamed to the new id, in every subject file
// the plan leaves in place, moved files included.
func (p *planner) rewriteLinks() error {
	mapping := map[string]string{}
	for _, r := range p.renames {
		if !strings.Contains(r.From, ":") {
			mapping[r.From] = r.To
		}
	}
	if len(mapping) == 0 {
		return nil
	}
	slugs := map[string][]string{}
	for _, st := range p.u.Stars() {
		slug := st.ID[strings.LastIndex(st.ID, "/")+1:]
		slugs[slug] = append(slugs[slug], st.ID)
	}
	resolve := func(l string) (string, bool) {
		if to, ok := mapping[l]; ok {
			return to, true
		}
		if ids := slugs[l]; len(ids) == 1 {
			if to, ok := mapping[ids[0]]; ok {
				return to, true
			}
		}
		return "", false
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
		m, _, err := parseFile(c)
		if err != nil || len(m.Links) == 0 {
			continue
		}
		self := st.ID
		if f, ok := p.files[rel]; ok && f.Kind == "move" {
			self = strings.TrimSuffix(strings.TrimPrefix(f.Path, kb.GalaxiesDir+"/"), ".md")
		}
		changed := false
		var out []string
		for _, l := range m.Links {
			if to, ok := resolve(l); ok {
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
		if !changed {
			continue
		}
		fields := map[string]any{"links": out}
		if len(out) == 0 {
			fields["links"] = []string{}
		}
		if c, err = setFields(c, fields); err != nil {
			return fmt.Errorf("restructure %s: %w", rel, err)
		}
		p.write(rel, c)
	}
	return nil
}

// --- apply ---

func (s *Service) applyPlan(ctx context.Context, plan Plan) error {
	var touched []string
	abs := func(rel string) string { return filepath.Join(s.root, filepath.FromSlash(rel)) }
	var emptied []string
	for _, f := range plan.Files {
		switch f.Kind {
		case "move":
			if err := os.MkdirAll(filepath.Dir(abs(f.Path)), 0o755); err != nil {
				return fmt.Errorf("restructure: %w", err)
			}
			if err := os.Rename(abs(f.From), abs(f.Path)); err != nil {
				return fmt.Errorf("restructure: move %s: %w", f.From, err)
			}
			if err := kb.WriteFile(abs(f.Path), []byte(f.Content)); err != nil {
				return err
			}
			touched = append(touched, abs(f.From), abs(f.Path))
			emptied = append(emptied, filepath.Dir(abs(f.From)))
		case "write":
			if err := os.MkdirAll(filepath.Dir(abs(f.Path)), 0o755); err != nil {
				return fmt.Errorf("restructure: %w", err)
			}
			if err := kb.WriteFile(abs(f.Path), []byte(f.Content)); err != nil {
				return err
			}
			touched = append(touched, abs(f.Path))
		case "delete":
			if err := os.Remove(abs(f.Path)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("restructure: delete %s: %w", f.Path, err)
			}
			touched = append(touched, abs(f.Path))
		}
	}
	for _, d := range emptied {
		os.Remove(d) // only succeeds when a moved sub-theme left its folder empty
	}
	if len(plan.Renames) > 0 {
		if err := s.book.AppendRenames(plan.Renames); err != nil {
			return err
		}
		touched = append(touched, s.book.RenamesFile())
	}
	if s.git != nil {
		if err := s.git.Commit(ctx, plan.Message, touched...); err != nil {
			return fmt.Errorf("restructure: files changed but not committed: %w", err)
		}
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
