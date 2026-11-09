package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/library"
)

// book add puts a document in the git-ignored library and prints the `source:` block the
// book-ingest skill copies into the fiche.
func book(args []string) error {
	if len(args) == 0 || args[0] != "add" {
		return errors.New("usage: rtk book add [--data DIR] [--title T] [--author A] [--year Y] FILE")
	}
	fs := flag.NewFlagSet("book add", flag.ExitOnError)
	data := dataFlag(fs)
	title := fs.String("title", "", "title of the work, defaults to the file name")
	author := fs.String("author", "", "author")
	year := fs.Int("year", 0, "year of publication")
	// The flag package stops at the first positional argument; keep parsing after it so
	// `rtk book add book.pdf --title X` works like `rtk book add --title X book.pdf`.
	var files []string
	for rest := args[1:]; ; {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		files = append(files, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(files) != 1 {
		return errors.New("usage: rtk book add [--data DIR] [--title T] [--author A] [--year Y] FILE")
	}
	if *data == "" {
		return errors.New("no knowledge folder: pass --data or set RTK_DATA")
	}
	src := files[0]
	e, err := library.Add(*data, src)
	if err != nil {
		return err
	}
	t := *title
	if t == "" {
		base := e.File[strings.LastIndex(e.File, "/")+1:]
		t = strings.TrimSuffix(base, base[strings.LastIndex(base, "-"):])
	}
	out, err := yaml.Marshal(map[string]kb.Source{"source": {Title: t, Author: *author, Year: *year, File: e.File, SHA256: e.SHA256}})
	if err != nil {
		return fmt.Errorf("book add: %w", err)
	}
	state := "added"
	if !e.Added {
		state = "already in the library"
	}
	fmt.Fprintf(os.Stderr, "%s %s (%d KiB), git-ignored\n", e.File, state, e.Size/1024)
	fmt.Print(string(out))
	return nil
}
