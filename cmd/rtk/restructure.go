package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
)

const restructureUsage = `rtk mv    [--data DIR] [--name N] [--dry-run] OLD NEW            move or rename a subject or sub-theme; NEW may be an existing sub-theme (or theme)
rtk split [--data DIR] [--dry-run] ID --into NEW:Notion[,Notion] [--into ...]
                                                                  move notions (with their cards) to new subjects; NEW is an id or a slug
rtk merge [--data DIR] [--name N] [--dry-run] A B [...] [--into C] fold subjects together, keep the highest level; C defaults to A
`

type multi []string

func (m *multi) String() string     { return strings.Join(*m, " ") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

// parseAll lets flags follow positional arguments, the way people type `rtk mv a b --dry-run`.
func parseAll(fs *flag.FlagSet, args []string) []string {
	var pos []string
	for {
		fs.Parse(args)
		if fs.NArg() == 0 {
			return pos
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func restructureCmd(ctx context.Context, log *slog.Logger, cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	data := dataFlag(fs)
	dry := fs.Bool("dry-run", false, "print the plan, change nothing")
	name := fs.String("name", "", "new display name")
	into := multi{}
	fs.Var(&into, "into", "split: NEW:Notion[,Notion] (repeat); merge: target id")
	fs.Usage = func() { fmt.Fprint(os.Stderr, restructureUsage) }
	pos := parseAll(fs, args)

	var op app.Op
	switch cmd {
	case "mv":
		if len(pos) != 2 {
			return errors.New("usage:\n" + restructureUsage)
		}
		op = app.Op{Kind: "move", ID: pos[0], To: pos[1], Name: *name}
	case "split":
		if len(pos) != 1 || len(into) == 0 {
			return errors.New("usage:\n" + restructureUsage)
		}
		op = app.Op{Kind: "split", ID: pos[0]}
		for _, part := range into {
			id, notions, ok := strings.Cut(part, ":")
			if !ok || id == "" || notions == "" {
				return fmt.Errorf("split: --into %q must be NEW:Notion[,Notion]", part)
			}
			var list []string
			for _, n := range strings.Split(notions, ",") {
				if n = strings.TrimSpace(n); n != "" {
					list = append(list, n)
				}
			}
			op.Parts = append(op.Parts, app.SplitPart{ID: id, Notions: list})
		}
	case "merge":
		if len(pos) < 2 && !(len(pos) == 1 && len(into) == 1) {
			return errors.New("usage:\n" + restructureUsage)
		}
		op = app.Op{Kind: "merge", Sources: pos, Name: *name}
		if len(into) > 0 {
			op.Into = into[len(into)-1]
		}
	}
	svc, err := open(ctx, log, *data)
	if err != nil {
		return err
	}
	plan, err := svc.Restructure(ctx, op, *dry, "")
	if err != nil {
		return err
	}
	for _, f := range plan.Files {
		switch f.Kind {
		case "move":
			fmt.Printf("  move    %s → %s\n", f.From, f.Path)
		default:
			fmt.Printf("  %-7s %s\n", f.Kind, f.Path)
		}
	}
	for _, r := range plan.Renames {
		fmt.Printf("  id      %s → %s\n", r.From, r.To)
	}
	if *dry {
		fmt.Println("dry run:", plan.Message)
		return nil
	}
	fmt.Println("done:", plan.Message)
	return nil
}
