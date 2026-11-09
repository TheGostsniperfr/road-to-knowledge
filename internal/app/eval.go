package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
)

// A query is either a paraphrase (no word in common with the fiche, the other language) or a
// name (a word the fiche contains, like "Cilium"). Words alone should win names; meaning should
// win paraphrases. Reporting them apart shows whether the fusion keeps the best of both.
const (
	KindParaphrase = "paraphrase"
	KindName       = "name"
)

// Thresholds are tuned on the "tune" set; the "heldout" set is only measured, so its numbers say
// how the search does on questions nobody tuned for.
const (
	SetTune    = "tune"
	SetHeldOut = "heldout"
)

// EvalQuery is one hand-written question with the subjects a good search must return.
type EvalQuery struct {
	Q    string
	Want []string
	Kind string
	Set  string
}

func evalQ(set, kind, question string, want ...string) EvalQuery {
	return EvalQuery{Q: question, Want: want, Kind: kind, Set: set}
}

// ExampleQueries exercise examples/knowledge.
var ExampleQueries = []EvalQuery{
	evalQ(SetTune, KindParaphrase, "isoler des conteneurs sans machine virtuelle", "tech/kubernetes/gvisor"),
	evalQ(SetTune, KindParaphrase, "sandbox for untrusted containers", "tech/kubernetes/gvisor"),
	evalQ(SetTune, KindParaphrase, "intercepting syscalls in user space", "tech/kubernetes/gvisor"),
	evalQ(SetTune, KindParaphrase, "consensus algorithm with leader election", "tech/donnees/raft"),
	evalQ(SetTune, KindParaphrase, "réplication validée à la majorité", "tech/donnees/raft"),
	evalQ(SetTune, KindParaphrase, "comment une base de données survit à une coupure de courant", "bibliotheque/tech/journal-avant-tout"),
	evalQ(SetTune, KindParaphrase, "write-ahead logging and durability", "bibliotheque/tech/journal-avant-tout"),
	evalQ(SetTune, KindParaphrase, "programmes chargés dans le noyau Linux", "tech/reseau/ebpf"),
	evalQ(SetTune, KindParaphrase, "kernel hooks for tracing and networking", "tech/reseau/ebpf"),
	evalQ(SetTune, KindParaphrase, "WebAssembly workloads on Kubernetes", "tech/kubernetes/runwasi"),
	evalQ(SetTune, KindParaphrase, "Japanese syllabaries", "langues/japonais/kana"),
	evalQ(SetTune, KindParaphrase, "caractères chinois de niveau intermédiaire", "langues/japonais/kanji-n3"),
	evalQ(SetTune, KindParaphrase, "grammar markers wa ga wo", "langues/japonais/particules"),
	evalQ(SetTune, KindParaphrase, "textbook for Japanese beginners", "bibliotheque/japonais/genki-1"),
	evalQ(SetTune, KindParaphrase, "écrire de la documentation et des tickets en anglais", "langues/anglais/anglais-technique"),
	evalQ(SetTune, KindParaphrase, "reading docs in English", "langues/anglais/anglais-technique"),
	evalQ(SetTune, KindName, "gVisor", "tech/kubernetes/gvisor"),
	evalQ(SetTune, KindName, "etcd", "tech/donnees/raft", "bibliotheque/tech/journal-avant-tout"),
	evalQ(SetTune, KindName, "how kubelet talks to containerd", "tech/kubernetes/container-runtimes"),
	evalQ(SetTune, KindName, "OCI runtime specification", "tech/kubernetes/container-runtimes"),
	evalQ(SetTune, KindName, "alphabet japonais hiragana katakana", "langues/japonais/kana"),
	evalQ(SetTune, KindName, "eBPF", "tech/reseau/ebpf"),
	evalQ(SetTune, KindName, "Raft", "tech/donnees/raft"),
	evalQ(SetTune, KindName, "runwasi", "tech/kubernetes/runwasi"),
	evalQ(SetTune, KindName, "Genki", "bibliotheque/japonais/genki-1"),
	evalQ(SetTune, KindName, "containerd shim", "tech/kubernetes/runwasi", "tech/kubernetes/container-runtimes"),

	evalQ(SetHeldOut, KindParaphrase, "faire tourner du code dans le noyau sans module", "tech/reseau/ebpf"),
	evalQ(SetHeldOut, KindParaphrase, "distributed agreement among replicas", "tech/donnees/raft"),
	evalQ(SetHeldOut, KindParaphrase, "garantir qu'une transaction validée n'est pas perdue", "bibliotheque/tech/journal-avant-tout"),
	evalQ(SetHeldOut, KindParaphrase, "the API a node agent uses to start containers", "tech/kubernetes/container-runtimes"),
	evalQ(SetHeldOut, KindParaphrase, "apprendre l'écriture phonétique du japonais", "langues/japonais/kana"),
	evalQ(SetHeldOut, KindParaphrase, "particles that mark the role of words in a sentence", "langues/japonais/particules"),
	evalQ(SetHeldOut, KindName, "runc", "tech/kubernetes/container-runtimes"),
	evalQ(SetHeldOut, KindName, "PostgreSQL", "bibliotheque/tech/journal-avant-tout"),
	evalQ(SetHeldOut, KindName, "ACID", "bibliotheque/tech/journal-avant-tout"),
	evalQ(SetHeldOut, KindName, "katakana", "langues/japonais/kana"),
	evalQ(SetHeldOut, KindName, "kanji N4", "langues/japonais/kanji-n3"),
	evalQ(SetHeldOut, KindName, "kubelet", "tech/kubernetes/container-runtimes"),
}

// ReadEvalQueries parses "question<TAB>id1,id2[<TAB>kind[<TAB>set]]" lines; # starts a comment.
// Kind defaults to paraphrase and set to tune.
func ReadEvalQueries(r io.Reader) ([]EvalQuery, error) {
	var out []EvalQuery
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 2 {
			return nil, fmt.Errorf("eval queries line %d: want question<TAB>id1,id2: %w", n, ErrInvalid)
		}
		e := EvalQuery{Q: strings.TrimSpace(cols[0]), Want: strings.Split(strings.ReplaceAll(cols[1], " ", ""), ","), Kind: KindParaphrase, Set: SetTune}
		if len(cols) > 2 && cols[2] != "" {
			e.Kind = strings.TrimSpace(cols[2])
		}
		if len(cols) > 3 && cols[3] != "" {
			e.Set = strings.TrimSpace(cols[3])
		}
		if (e.Kind != KindParaphrase && e.Kind != KindName) || (e.Set != SetTune && e.Set != SetHeldOut) {
			return nil, fmt.Errorf("eval queries line %d: kind is paraphrase|name, set is tune|heldout: %w", n, ErrInvalid)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

type EvalRow struct {
	Strategy  string   `json:"strategy"`
	Set       string   `json:"set"`
	Kind      string   `json:"kind"`
	N         int      `json:"n"`
	RecallAt5 float64  `json:"recall_at_5"`
	MRR       float64  `json:"mrr"`
	Misses    []string `json:"misses,omitempty"`
}

type EvalReport struct {
	Rows []EvalRow `json:"rows"`
	// Noise is the average number of hybrid results for questions the repo cannot answer.
	Noise float64 `json:"noise"`
}

// offTopic are questions the repo cannot answer: a good search returns nothing for them.
var offTopic = []string{"zzqx wobble frumious", "recette de la blanquette de veau", "résultats du championnat de curling", "how to knit a sweater"}

var strategies = []struct {
	name string
	how  Strategy
}{{"all words", AllWordsOnly}, {"words", WordsOnly}, {"meaning", MeaningOnly}, {"hybrid", Hybrid}}

// EvalSearch scores every strategy per set and kind. Recall@5 counts a question as found when an
// expected subject is in the first five; MRR rewards finding it first.
func (s *Service) EvalSearch(ctx context.Context, queries []EvalQuery) (EvalReport, error) {
	var r EvalReport
	for _, st := range strategies {
		for _, set := range []string{SetTune, SetHeldOut} {
			for _, kind := range []string{KindParaphrase, KindName, ""} {
				row := EvalRow{Strategy: st.name, Set: set, Kind: kind}
				if kind == "" {
					row.Kind = "all"
				}
				for _, e := range queries {
					if e.Set != set || (kind != "" && e.Kind != kind) {
						continue
					}
					hits, err := s.SearchWith(ctx, e.Q, 10, st.how)
					if err != nil {
						return r, err
					}
					row.N++
					rank := 0
					for i, h := range hits {
						if slices.Contains(e.Want, h.ID) {
							rank = i + 1
							break
						}
					}
					if rank > 0 && rank <= 5 {
						row.RecallAt5++
					} else {
						row.Misses = append(row.Misses, e.Q)
					}
					if rank > 0 {
						row.MRR += 1 / float64(rank)
					}
				}
				if row.N == 0 {
					continue
				}
				row.RecallAt5 /= float64(row.N)
				row.MRR /= float64(row.N)
				r.Rows = append(r.Rows, row)
			}
		}
	}
	for _, q := range offTopic {
		hits, err := s.SearchWith(ctx, q, 10, Hybrid)
		if err != nil {
			return r, err
		}
		r.Noise += float64(len(hits)) / float64(len(offTopic))
	}
	return r, nil
}

// Row finds one line of the report; ok is false when the set had no such query.
func (r EvalReport) Row(strategy, set, kind string) (EvalRow, bool) {
	for _, row := range r.Rows {
		if row.Strategy == strategy && row.Set == set && row.Kind == kind {
			return row, true
		}
	}
	return EvalRow{}, false
}
