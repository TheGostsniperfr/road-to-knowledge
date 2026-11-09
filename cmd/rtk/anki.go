package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/anki"
)

const ankiUsage = `rtk anki export [--data DIR] [--scope ID] -o FILE.apkg   cards to an Anki package
rtk anki import [--data DIR] [--propose] FILE.apkg      new cards to inbox/; with media or --propose, to proposals/anki-import
rtk anki push   [--data DIR] [--scope ID] [--url URL]   add or update notes in a running Anki (AnkiConnect)
`

func ankiCmd(ctx context.Context, log *slog.Logger, args []string) error {
	if len(args) == 0 {
		return errors.New("usage:\n" + ankiUsage)
	}
	fs := flag.NewFlagSet("anki "+args[0], flag.ExitOnError)
	data := dataFlag(fs)
	scope := fs.String("scope", "", "galaxy, sub-theme or subject id; empty for everything")
	out := fs.String("o", "", "output .apkg")
	propose := fs.Bool("propose", false, "write on branch proposals/anki-import and put cards back in their subjects")
	url := fs.String("url", anki.DefaultConnectURL, "AnkiConnect address, localhost only")
	fs.Parse(args[1:])
	svc, err := open(ctx, log, *data)
	if err != nil {
		return err
	}
	warnDuplicates := func() error {
		_, dups, err := svc.AnkiNotes(*scope)
		for _, d := range dups {
			fmt.Fprintf(os.Stderr, "rtk: %s asks %q twice; only the first card goes to Anki\n", d.StarID, d.Question)
		}
		return err
	}
	switch args[0] {
	case "export":
		if err := warnDuplicates(); err != nil {
			return err
		}
		if *out == "" {
			return errors.New("anki export: -o FILE.apkg is required")
		}
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		if _, err := svc.AnkiExport(ctx, *scope, f); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Println("wrote", *out)
	case "import":
		if fs.NArg() != 1 {
			return errors.New("anki import: one .apkg file expected")
		}
		st, branch, err := svc.AnkiImport(ctx, fs.Arg(0), *propose)
		if err != nil {
			return err
		}
		fmt.Printf("%d notes, %d cards (%d from cloze notes): %d already known, %d to inbox, %d back to their subject\n",
			st.Notes, st.Cards, st.Cloze, st.Known, st.ToInbox, st.ToSubjects)
		fmt.Printf("media: %d copied to assets/anki, %d skipped; %d notes not understood\n", st.MediaCopied, st.MediaSkipped, st.Unsupported)
		if branch != "" {
			if !*propose {
				fmt.Println("media found: images and sounds never go to main, so the import went to a proposal branch")
			}
			fmt.Printf("review with: git diff main...%s\n", branch)
		}
	case "push":
		if err := warnDuplicates(); err != nil {
			return err
		}
		notes, _, err := svc.AnkiNotes(*scope)
		if err != nil {
			return err
		}
		c, err := anki.NewConnect(*url)
		if err != nil {
			return err
		}
		st, err := c.Push(ctx, notes)
		if errors.Is(err, anki.ErrAnkiNotRunning) {
			fmt.Fprintln(os.Stderr, "rtk: Anki is not reachable. Open Anki desktop with the AnkiConnect add-on (code 2055492159), then run this again.")
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Printf("%d added, %d updated, %d unchanged\n", st.Added, st.Updated, st.Unchanged)
	default:
		return errors.New("usage:\n" + ankiUsage)
	}
	return nil
}
