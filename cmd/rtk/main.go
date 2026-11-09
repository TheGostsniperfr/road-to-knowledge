// Command rtk serves Road to Knowledge: the web app, the JSON API and the MCP server over one knowledge folder.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/httpapi"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/mcpserver"
	"github.com/TheGostsniperfr/road-to-knowledge/web"
)

var version = "dev"

const usage = `rtk: Road to Knowledge

  rtk serve    [--data DIR] [--addr 127.0.0.1:7878] [--news-every 6h]
                                                      web app + API + MCP over HTTP at /mcp
  rtk mcp      [--data DIR]                           MCP over stdio, for Claude Code
  rtk news     fetch|list [--data DIR] [--scope ID] [--since 7d] [--all]
                                                      fetch or read the feeds of your themes
  rtk validate [--data DIR]                           check every file, exit 1 on problems
  rtk init     DIR                                    create an empty knowledge folder
  rtk book add [--data DIR] [--title T] FILE          put a PDF in the git-ignored library, print its source block

DIR defaults to $RTK_DATA.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// stdout belongs to the MCP protocol in `rtk mcp`, so logs always go to stderr.
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(ctx, log, os.Args[2:])
	case "mcp":
		err = stdio(ctx, log, os.Args[2:])
	case "news":
		err = newsCmd(ctx, log, os.Args[2:])
	case "validate":
		err = validate(os.Args[2:])
	case "init":
		err = initData(os.Args[2:])
	case "book":
		err = book(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "rtk:", err)
		os.Exit(1)
	}
}

func dataFlag(fs *flag.FlagSet) *string {
	return fs.String("data", os.Getenv("RTK_DATA"), "knowledge folder")
}

func open(ctx context.Context, log *slog.Logger, dir string) (*app.Service, error) {
	if dir == "" {
		return nil, errors.New("no knowledge folder: pass --data or set RTK_DATA")
	}
	return app.New(ctx, dir, log)
}

func serve(ctx context.Context, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	data := dataFlag(fs)
	addr := fs.String("addr", "127.0.0.1:7878", "listen address, keep it on localhost")
	newsEvery := fs.Duration("news-every", 6*time.Hour, "fetch the theme feeds this often, 0 to disable")
	fs.Parse(args)
	svc, err := open(ctx, log, *data)
	if err != nil {
		return err
	}
	go svc.RunNews(ctx, *newsEvery)
	go func() {
		if err := svc.Watch(ctx); err != nil {
			log.Error("watch stopped", "err", err)
		}
	}()
	dist := web.Dist()
	if dist == nil {
		log.Warn("web app not built: run `npm run build` in web/, or use `npm run dev` for development")
	}
	h := httpapi.New(svc, log, dist, mcpserver.New(svc, version)).Handler()
	return httpapi.Serve(ctx, *addr, h, log)
}

func stdio(ctx context.Context, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	data := dataFlag(fs)
	fs.Parse(args)
	svc, err := open(ctx, log, *data)
	if err != nil {
		return err
	}
	go svc.Watch(ctx)
	return mcpserver.New(svc, version).Run(ctx, &mcp.StdioTransport{})
}

func validate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	data := dataFlag(fs)
	fs.Parse(args)
	if *data == "" {
		return errors.New("no knowledge folder: pass --data or set RTK_DATA")
	}
	u, problems, err := kb.Load(*data)
	if err != nil {
		return err
	}
	for _, p := range problems {
		fmt.Printf("%s: %s\n", p.Path, p.Message)
	}
	fmt.Printf("%d galaxies, %d subjects, %d cards, %d problems\n", len(u.Galaxies), len(u.Stars()), len(u.Cards()), len(problems))
	if len(problems) > 0 {
		os.Exit(1)
	}
	return nil
}

func initData(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: rtk init DIR")
	}
	dir := args[0]
	if entries, err := os.ReadDir(filepath.Join(dir, kb.GalaxiesDir)); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s already holds galaxies", dir)
	}
	files := map[string]string{
		".gitignore":                              ".rtk/\n*.tmp\nlibrary/\n",
		"review/.gitkeep":                         "",
		"inbox/.gitkeep":                          "",
		"assets/.gitkeep":                         "",
		"galaxies/exemple/_galaxy.md":             "---\nname: Exemple\nshape: spiral\nreview_after_days: 90\n---\n",
		"galaxies/exemple/debut/_cluster.md":      "---\nname: Début\n---\n",
		"galaxies/exemple/debut/premier-sujet.md": "---\nname: Premier sujet\nlevel: 1\n---\nUne ligne qui résume le sujet.\n\nUne question :: sa réponse\n\n## Une notion\nCe qu'il faut en retenir.\n",
	}
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("knowledge folder ready in %s\n", dir)
	return nil
}
