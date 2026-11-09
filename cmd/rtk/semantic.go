package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/embed"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/semantic"
)

func modelFlag(fs *flag.FlagSet, def string) *string {
	return fs.String("model", def, `embedding model for search by meaning (`+embed.Default+` or embeddinggemma-300m), "off" for words only`)
}

func opener(id string) app.Opener {
	return func(ctx context.Context) (embed.Embedder, semantic.Thresholds, error) {
		h, err := embed.Open(ctx, id)
		if err != nil {
			return nil, semantic.Thresholds{}, err
		}
		sp := h.Spec()
		return h, semantic.Thresholds{Search: sp.MinScore, Near: sp.NearScore}, nil
	}
}

// enableSemantic starts loading the model in the background and returns the function that stops
// the build and releases the model, to defer once the server is done.
func enableSemantic(ctx context.Context, svc *app.Service, model string) func() {
	if model == "off" {
		return func() {}
	}
	svc.EnableSemantic(ctx, opener(model))
	return svc.StopSemantic
}

const modelsUsage = `rtk models pull [--model ID] [--verify]   download a pinned embedding model to the user cache;
                                         --verify rehashes what is there and repairs it
rtk models list                          list known models and whether they are downloaded
`

func modelsCmd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, modelsUsage)
		return errors.New("missing subcommand")
	}
	fs := flag.NewFlagSet("models", flag.ExitOnError)
	model := modelFlag(fs, embed.Default)
	verify := fs.Bool("verify", false, "rehash downloaded files and download again those that do not match")
	fs.Parse(args[1:])
	switch args[0] {
	case "list":
		dir, err := embed.ModelsDir()
		if err != nil {
			return err
		}
		fmt.Println("models in", dir)
		for _, id := range []string{"multilingual-e5-small", "embeddinggemma-300m"} {
			sp := embed.Models[id]
			state := "downloaded"
			if sp.Present() != nil {
				state = "not downloaded"
			}
			def := ""
			if id == embed.Default {
				def = " (default)"
			}
			fmt.Printf("  %-24s %6.0f MB  %-20s %s%s\n", id, float64(sp.Size())/1e6, sp.License, state, def)
		}
		return nil
	case "pull":
		sp, err := embed.Lookup(*model)
		if err != nil {
			return err
		}
		if *verify {
			bad, err := sp.Verify()
			if err != nil {
				return err
			}
			for _, f := range bad {
				fmt.Fprintln(os.Stderr, "corrupt, downloading again:", f)
			}
		}
		fmt.Fprintf(os.Stderr, "downloading %s (%.0f MB, license %s)\n", sp.ID, float64(sp.Size())/1e6, sp.License)
		last := time.Time{}
		err = sp.Pull(ctx, func(file string, done, total int64) {
			if time.Since(last) > time.Second || done == total {
				last = time.Now()
				fmt.Fprintf(os.Stderr, "\r  %-28s %5.1f%%", file, 100*float64(done)/float64(max(total, 1)))
				if done == total {
					fmt.Fprintln(os.Stderr)
				}
			}
		})
		if err != nil {
			return err
		}
		dir, _ := sp.Dir()
		fmt.Println("ready in", dir)
		return nil
	default:
		fmt.Fprint(os.Stderr, modelsUsage)
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func benchCmd(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "search" {
		return errors.New("usage: rtk bench search [--data DIR] [--model ID] [--queries FILE.tsv] [--json]")
	}
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	data := dataFlag(fs)
	model := modelFlag(fs, embed.Default)
	queriesFile := fs.String("queries", "", "question<TAB>id1,id2[<TAB>paraphrase|name[<TAB>tune|heldout]] per line; default: the examples set")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	fs.Parse(args[1:])
	queries := app.ExampleQueries
	if *queriesFile != "" {
		f, err := os.Open(*queriesFile)
		if err != nil {
			return err
		}
		queries, err = app.ReadEvalQueries(f)
		f.Close()
		if err != nil {
			return err
		}
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := open(ctx, quiet, *data)
	if err != nil {
		return err
	}
	start := time.Now()
	defer enableSemantic(ctx, svc, *model)()
	if err := svc.WaitSemantic(ctx); err != nil {
		return err
	}
	st := svc.SemanticStatus()
	if *model != "off" && st.State != semantic.StateReady {
		return fmt.Errorf("search by meaning is %s: %s", st.State, st.Message)
	}
	indexed := time.Since(start)
	t0 := time.Now()
	probe := 0
	for _, q := range queries {
		if _, err := svc.Search(ctx, q.Q, 10); err != nil {
			return err
		}
		probe++
	}
	perQuery := time.Since(t0) / time.Duration(max(probe, 1))
	r, err := svc.EvalSearch(ctx, queries)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(r)
	}
	fmt.Printf("model %s, %d chunks, load+index %s, %s per hybrid search\n", st.Model, st.Total, indexed.Round(time.Millisecond), perQuery.Round(time.Millisecond))
	fmt.Printf("%-10s %-8s %-11s %3s  %8s  %5s\n", "strategy", "set", "kind", "n", "recall@5", "MRR")
	for _, row := range r.Rows {
		fmt.Printf("%-10s %-8s %-11s %3d  %8.2f  %5.2f\n", row.Strategy, row.Set, row.Kind, row.N, row.RecallAt5, row.MRR)
	}
	fmt.Printf("off-topic questions return %.1f hybrid results on average (lower is better)\n", r.Noise)
	for _, set := range []string{app.SetTune, app.SetHeldOut} {
		if row, ok := r.Row("hybrid", set, "all"); ok {
			for _, m := range row.Misses {
				fmt.Printf("hybrid miss (%s): %s\n", set, m)
			}
		}
	}
	return nil
}
