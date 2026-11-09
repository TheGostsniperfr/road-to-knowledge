// Package kb loads the knowledge repository: one Markdown file per node, hierarchy given by directories.
package kb

import "time"

type Kind string

const (
	KindGalaxy  Kind = "galaxy"
	KindCluster Kind = "cluster"
	KindStar    Kind = "star"
)

type State string

const (
	StateIdea     State = "idea"
	StateExplore  State = "explore"
	StateProgress State = "progress"
	StateAcquired State = "acquired"
)

func (s State) Valid() bool {
	switch s {
	case StateIdea, StateExplore, StateProgress, StateAcquired:
		return true
	}
	return false
}

var Shapes = map[string]bool{"spiral": true, "barred": true, "ring": true, "elliptical": true, "lenticular": true, "irregular": true}

type Discovery struct {
	Near     int  `yaml:"near" json:"near"`
	TwoHops  int  `yaml:"two_hops" json:"two_hops"`
	OffPiste int  `yaml:"off_piste" json:"off_piste"`
	News     bool `yaml:"news" json:"news"`
}

// Valid requires a full budget: the three shares are percentages of the same suggestions.
func (d Discovery) Valid() bool {
	return d.Near >= 0 && d.TwoHops >= 0 && d.OffPiste >= 0 && d.Near+d.TwoHops+d.OffPiste == 100
}

type Galaxy struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Hue         float64    `json:"hue"`
	Shape       string     `json:"shape"`
	ReviewAfter int        `json:"review_after_days"`
	Discovery   Discovery  `json:"discovery"`
	Pin         *Pin       `json:"pin,omitempty"`
	Body        string     `json:"-"`
	Path        string     `json:"-"`
	Clusters    []*Cluster `json:"-"`
}

type Cluster struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Body   string  `json:"-"`
	Path   string  `json:"-"`
	Galaxy *Galaxy `json:"-"`
	Stars  []*Star `json:"-"`
}

type Planet struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Body string `json:"body"`
}

type Card struct {
	ID       string `json:"id"`
	StarID   string `json:"star"`
	Question string `json:"q"`
	Answer   string `json:"a"`
}

type Star struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Level    int       `json:"level"`
	State    State     `json:"state"`
	Links    []string  `json:"links,omitempty"`
	Evidence []string  `json:"evidence,omitempty"`
	Tags     []string  `json:"tags,omitempty"`
	Reviewed time.Time `json:"reviewed"`
	Summary  string    `json:"summary"`
	Body     string    `json:"-"`
	Path     string    `json:"-"`
	Source   *Source   `json:"source,omitempty"`
	Planets  []Planet  `json:"planets,omitempty"`
	Cards    []Card    `json:"-"`
	Cluster  *Cluster  `json:"-"`
}

// Problem is a validation finding; loading continues past problems so one bad file never hides the universe.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type Universe struct {
	Root     string
	Galaxies []*Galaxy
	galaxies map[string]*Galaxy
	clusters map[string]*Cluster
	stars    map[string]*Star
}

func (u *Universe) Galaxy(id string) *Galaxy   { return u.galaxies[id] }
func (u *Universe) Cluster(id string) *Cluster { return u.clusters[id] }
func (u *Universe) Star(id string) *Star       { return u.stars[id] }

func (u *Universe) Stars() []*Star {
	out := make([]*Star, 0, len(u.stars))
	for _, g := range u.Galaxies {
		for _, c := range g.Clusters {
			out = append(out, c.Stars...)
		}
	}
	return out
}

func (u *Universe) Cards() []Card {
	var out []Card
	for _, s := range u.Stars() {
		out = append(out, s.Cards...)
	}
	return out
}
