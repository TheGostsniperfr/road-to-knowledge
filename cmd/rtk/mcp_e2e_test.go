package main_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests drive the real binary the way Claude Code does: a child process over stdio, and the
// streamable HTTP endpoint of `rtk serve`. They catch what in-memory transports cannot: flags, logs
// on stdout, the Host/Origin guard, and the route itself.

var (
	binPath  string
	buildErr error
)

// TestMain builds rtk once for every test in this package.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rtk-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binPath = filepath.Join(dir, "rtk")
	if out, err := exec.Command("go", "build", "-o", binPath, ".").CombinedOutput(); err != nil {
		buildErr = fmt.Errorf("go build: %w: %s", err, out)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func buildBinary(t *testing.T) string {
	t.Helper()
	require.NoError(t, buildErr)
	return binPath
}

// copyExamples copies examples/knowledge into a fresh git repository, so propose() has a main branch.
func copyExamples(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	src := filepath.Join("..", "..", "examples", "knowledge")
	require.NoError(t, os.CopyFS(dst, os.DirFS(src)))
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@e.st"}, {"config", "user.name", "t"}, {"config", "commit.gpgsign", "false"}, {"add", "."}, {"commit", "-qm", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dst
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return dst
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

type client struct {
	t  *testing.T
	cs *mcp.ClientSession
}

func (c client) call(name string, args map[string]any) (string, bool) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := c.cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(c.t, err, name)
	require.NotEmpty(c.t, res.Content, name)
	return res.Content[0].(*mcp.TextContent).Text, res.IsError
}

func stdioClient(t *testing.T, bin, data string) client {
	t.Helper()
	ctx := context.Background()
	cmd := exec.Command(bin, "mcp", "--data", data)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "e2e"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close() })
	return client{t, cs}
}

func httpClient(t *testing.T, bin, data string) client {
	t.Helper()
	addr := freeAddr(t)
	cmd := exec.Command(bin, "serve", "--data", data, "--addr", addr)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	deadline := time.Now().Add(10 * time.Second)
	for {
		res, err := http.Get("http://" + addr + "/api/universe")
		if err == nil {
			res.Body.Close()
			break
		}
		require.True(t, time.Now().Before(deadline), "server did not start: %v", err)
		time.Sleep(100 * time.Millisecond)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "e2e"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: "http://" + addr + "/mcp"}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close() })
	return client{t, cs}
}

func TestMCP_EveryToolAnswers_OverStdioAndHTTP(t *testing.T) {
	bin := buildBinary(t)
	for name, connect := range map[string]func(*testing.T, string, string) client{"stdio": stdioClient, "http": httpClient} {
		t.Run(name, func(t *testing.T) {
			data := copyExamples(t)
			c := connect(t, bin, data)

			tools, err := c.cs.ListTools(context.Background(), nil)
			require.NoError(t, err)
			var names []string
			for _, tl := range tools.Tools {
				names = append(names, tl.Name)
				assert.NotEmpty(t, tl.Description, tl.Name)
			}
			t.Logf("tools: %s", strings.Join(names, ", "))
			assert.Contains(t, c.cs.InitializeResult().Instructions, "profile()")

			calls := []struct {
				tool string
				args map[string]any
				want string
			}{
				{"profile", nil, `"themes"`},
				{"subgraph", map[string]any{"id": "tech", "depth": 2}, `"children"`},
				{"search", map[string]any{"query": "kana"}, "kana"},
				{"node", map[string]any{"id": "langues/japonais/kana"}, `"summary"`},
				{"fragments", map[string]any{"id": "langues/japonais/kana"}, "::"},
				{"frontier", nil, `"suggestions"`},
				{"due", nil, `"cards"`},
				{"at_risk", nil, "["},
				{"recent", map[string]any{"days": 30}, `"commits"`},
				{"inbox", nil, `"notes"`},
				{"capture", map[string]any{"text": "lire la doc de Cilium", "source": "e2e"}, "inbox/"},
				{"propose", map[string]any{"slug": "e2e", "message": "feat(seed): e2e", "files": []map[string]any{{"path": "galaxies/tech/k8s/e2e.md", "content": "---\nname: E2E\nlevel: 0\n---\nTest.\n"}}}, "proposals/e2e"},
			}
			for _, cl := range calls {
				out, isErr := c.call(cl.tool, cl.args)
				assert.False(t, isErr, "%s: %s", cl.tool, out)
				assert.Contains(t, out, cl.want, cl.tool)
				t.Logf("%-9s %5d bytes ≈ %4d tokens", cl.tool, len(out), len(out)/4)
			}
		})
	}
}

func TestMCP_ErrorsSayWhatToDoNext_OverStdio(t *testing.T) {
	bin := buildBinary(t)
	c := stdioClient(t, bin, copyExamples(t))

	cases := []struct {
		tool string
		args map[string]any
		hint string
	}{
		{"node", map[string]any{"id": "nope/nope/nope"}, "search"},
		{"subgraph", map[string]any{"id": "nope"}, "profile"},
		{"fragments", map[string]any{"id": "langues/japonais/kana", "section": "Nope"}, "sections:"},
		{"frontier", map[string]any{"scope": "nope"}, "profile"},
		{"propose", map[string]any{"slug": "x", "message": "m", "files": []map[string]any{{"path": "../etc/passwd", "content": "x"}}}, "galaxies/"},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			out, isErr := c.call(tc.tool, tc.args)

			assert.True(t, isErr, out)
			assert.Contains(t, out, tc.hint)
		})
	}
}

func TestMCP_StdoutCarriesOnlyProtocol(t *testing.T) {
	bin := buildBinary(t)
	data := copyExamples(t)
	cmd := exec.Command(bin, "mcp", "--data", data)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	_, err = io.WriteString(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"raw","version":"0"}}}`+"\n")
	require.NoError(t, err)
	first := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		first <- line
	}()

	select {
	case line := <-first:
		assert.True(t, strings.HasPrefix(line, `{"jsonrpc"`), fmt.Sprintf("first stdout line is not the protocol: %q", line))
	case <-time.After(10 * time.Second):
		t.Fatal("no answer to initialize on stdout")
	}
}
