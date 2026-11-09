package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
)

// EvalQuery is one hand-written question with the subjects a good search must return.
type EvalQuery struct {
	Q    string
	Want []string
}

// ExampleQueries exercise examples/knowledge: paraphrases with no shared word, the other language,
// and proper names that words alone already find.
var ExampleQueries = []EvalQuery{
	{"isoler des conteneurs sans machine virtuelle", []string{"tech/kubernetes/gvisor"}},
	{"sandbox for untrusted containers", []string{"tech/kubernetes/gvisor"}},
	{"intercepting syscalls in user space", []string{"tech/kubernetes/gvisor"}},
	{"gVisor", []string{"tech/kubernetes/gvisor"}},
	{"consensus algorithm with leader election", []string{"tech/donnees/raft"}},
	{"réplication validée à la majorité", []string{"tech/donnees/raft"}},
	{"etcd", []string{"tech/donnees/raft", "bibliotheque/tech/journal-avant-tout"}},
	{"comment une base de données survit à une coupure de courant", []string{"bibliotheque/tech/journal-avant-tout"}},
	{"write-ahead logging and durability", []string{"bibliotheque/tech/journal-avant-tout"}},
	{"programmes chargés dans le noyau Linux", []string{"tech/reseau/ebpf"}},
	{"kernel hooks for tracing and networking", []string{"tech/reseau/ebpf"}},
	{"WebAssembly workloads on Kubernetes", []string{"tech/kubernetes/runwasi"}},
	{"how kubelet talks to containerd", []string{"tech/kubernetes/container-runtimes"}},
	{"OCI runtime specification", []string{"tech/kubernetes/container-runtimes"}},
	{"Japanese syllabaries", []string{"langues/japonais/kana"}},
	{"alphabet japonais hiragana katakana", []string{"langues/japonais/kana"}},
	{"caractères chinois de niveau intermédiaire", []string{"langues/japonais/kanji-n3"}},
	{"grammar markers wa ga wo", []string{"langues/japonais/particules"}},
	{"textbook for Japanese beginners", []string{"bibliotheque/japonais/genki-1"}},
	{"écrire de la documentation et des tickets en anglais", []string{"langues/anglais/anglais-technique"}},
	{"reading docs in English", []string{"langues/anglais/anglais-technique"}},
}

// ReadEvalQueries parses "question<TAB>id1,id2" lines; # starts a comment.
func ReadEvalQueries(r io.Reader) ([]EvalQuery, error) {
	var out []EvalQuery
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		q, ids, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("eval queries line %d: want question<TAB>id1,id2: %w", n, ErrInvalid)
		}
		out = append(out, EvalQuery{Q: strings.TrimSpace(q), Want: strings.Split(strings.ReplaceAll(ids, " ", ""), ",")})
	}
	return out, sc.Err()
}

type EvalScore struct {
	RecallAt5 float64  `json:"recall_at_5"`
	MRR       float64  `json:"mrr"`
	Misses    []string `json:"misses,omitempty"`
}

// offTopic are questions the repo cannot answer: a good search returns nothing for them.
var offTopic = []string{"zzqx wobble frumious", "recette de la blanquette de veau", "résultats du championnat de curling", "how to knit a sweater"}

type EvalReport struct {
	Queries int `json:"queries"`
	// Noise is the average number of results for off-topic questions.
	Noise   float64   `json:"noise"`
	Words   EvalScore `json:"words"`
	Meaning EvalScore `json:"meaning"`
	Hybrid  EvalScore `json:"hybrid"`
}

// EvalSearch scores words only, meaning only and the fusion on the same questions. Recall@5 counts a
// question as found when an expected subject is in the first five; MRR rewards finding it first.
func (s *Service) EvalSearch(ctx context.Context, queries []EvalQuery) (EvalReport, error) {
	r := EvalReport{Queries: len(queries)}
	modes := []struct {
		out          *EvalScore
		words, sense bool
	}{{&r.Words, true, false}, {&r.Meaning, false, true}, {&r.Hybrid, true, true}}
	for _, m := range modes {
		for _, q := range queries {
			hits, err := s.SearchWith(ctx, q.Q, 10, m.words, m.sense)
			if err != nil {
				return r, err
			}
			rank := 0
			for i, h := range hits {
				if slices.Contains(q.Want, h.ID) {
					rank = i + 1
					break
				}
			}
			if rank > 0 && rank <= 5 {
				m.out.RecallAt5++
			} else {
				m.out.Misses = append(m.out.Misses, q.Q)
			}
			if rank > 0 {
				m.out.MRR += 1 / float64(rank)
			}
		}
		if len(queries) > 0 {
			m.out.RecallAt5 /= float64(len(queries))
			m.out.MRR /= float64(len(queries))
		}
	}
	for _, q := range offTopic {
		hits, err := s.SearchWith(ctx, q, 10, true, true)
		if err != nil {
			return r, err
		}
		r.Noise += float64(len(hits)) / float64(len(offTopic))
	}
	return r, nil
}
