package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
)

func newsCmd(ctx context.Context, log *slog.Logger, args []string) error {
	if len(args) == 0 || (args[0] != "fetch" && args[0] != "list") {
		return errors.New("usage: rtk news fetch|list [--data DIR] [--scope ID] [--since 7d] [--all]")
	}
	fs := flag.NewFlagSet("news", flag.ExitOnError)
	data := dataFlag(fs)
	scope := fs.String("scope", "", "theme or sub-theme id")
	since := fs.String("since", "7d", "only items after: 2026-11-09, RFC 3339, 24h or 7d")
	all := fs.Bool("all", false, "include items that mention no known subject")
	fs.Parse(args[1:])
	svc, err := open(ctx, log, *data)
	if err != nil {
		return err
	}
	if args[0] == "fetch" {
		rep, err := svc.FetchNews(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("%d feeds fetched, %d unchanged, %d skipped (backoff), %d failed, %d new items\n", rep.Fetched, rep.NotMod, rep.Skipped, rep.Failed, rep.NewItems)
		for _, e := range rep.Errors {
			fmt.Println("  failed:", e)
		}
		return nil
	}
	t, err := app.ParseSince(*since, time.Now())
	if err != nil {
		return err
	}
	v, err := svc.News(app.NewsQuery{Scope: *scope, Since: t, Limit: 200, All: *all})
	if err != nil {
		return err
	}
	for _, it := range v.Items {
		var subj []string
		for _, m := range it.Subjects {
			subj = append(subj, fmt.Sprintf("%s (%d)", m.Name, m.Level))
		}
		fmt.Printf("%s  %-18s %s\n            %s", it.Published.Format("2006-01-02"), clipName(it.Source), it.Title, it.Link)
		if len(subj) > 0 {
			fmt.Printf("\n            sur %s", strings.Join(subj, ", "))
		}
		fmt.Println()
	}
	if v.Hidden > 0 {
		fmt.Printf("%d items mention none of your subjects; --all shows them\n", v.Hidden)
	}
	return nil
}

func clipName(s string) string {
	if r := []rune(s); len(r) > 18 {
		return string(r[:17]) + "…"
	}
	return s
}
