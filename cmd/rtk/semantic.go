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

func modelFlag(fs *flag.FlagSet) *string {
	return fs.String("model", embed.Default, `embedding model for search by meaning, "off" to disable`)
}

func opener(id string) app.Opener {
	return func(ctx context.Context) (embed.Embedder, semantic.Thresholds, error) {
		h, err := embed.Open(ctx, id)
		if err != nil {
			return nil, semantic.Thresholds{}, err
		}
		go func() {
			<-ctx.Done()
			h.Close()
		}()
		sp := h.Spec()
		return h, semantic.Thresholds{Search: sp.MinScore, Near: sp.NearScore}, nil
	}
}

func enableSemantic(ctx context.Context, svc *app.Service, model string) {
	if model != "off" {
		svc.EnableSemantic(ctx, opener(model))
	}
}

const modelsUsage = `rtk models pull [--model ID]   download a pinned embedding model to the user cache
rtk models list                list known models and whether they are downloaded
`

func modelsCmd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, modelsUsage)
		return errors.New("missing subcommand")
	}
	fs := flag.NewFlagSet("models", flag.ExitOnError)
	model := modelFlag(fs)
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

func benchCmd(ctx context.Context, log *slog.Logger, args []string) error {
	if len(args) == 0 || args[0] != "search" {
		return errors.New("usage: rtk bench search [--data DIR] [--model ID] [--queries FILE.tsv] [--json]")
	}
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	data := dataFlag(fs)
	model := modelFlag(fs)
	queriesFile := fs.String("queries", "", "question<TAB>id1,id2 per line; default: the examples set")
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
	svc.EnableSemantic(ctx, opener(*model))
	if err := svc.WaitSemantic(ctx); err != nil {
		return err
	}
	st := svc.SemanticStatus()
	if st.State != semantic.StateReady {
		return fmt.Errorf("search by meaning is %s: %s", st.State, st.Message)
	}
	indexed := time.Since(start)
	t0 := time.Now()
	r, err := svc.EvalSearch(ctx, queries)
	if err != nil {
		return err
	}
	perQuery := time.Since(t0) / time.Duration(3*max(len(queries), 1))
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(r)
	}
	fmt.Printf("model %s, %d chunks, load+index %s, %s per search\n", st.Model, st.Total, indexed.Round(time.Millisecond), perQuery.Round(time.Millisecond))
	fmt.Printf("%d queries      recall@5   MRR\n", r.Queries)
	for _, row := range []struct {
		name string
		s    app.EvalScore
	}{{"words  ", r.Words}, {"meaning", r.Meaning}, {"hybrid ", r.Hybrid}} {
		fmt.Printf("  %s       %5.2f    %5.2f\n", row.name, row.s.RecallAt5, row.s.MRR)
	}
	fmt.Printf("  off-topic questions return %.1f results on average (lower is better)\n", r.Noise)
	for _, m := range r.Hybrid.Misses {
		fmt.Println("  hybrid miss:", m)
	}
	return nil
}
