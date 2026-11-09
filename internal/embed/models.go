package embed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
)

// File is one file of a model, pinned by revision and hash so a download can never change under us.
type File struct {
	Path   string
	SHA256 string
	Size   int64
}

type Spec struct {
	ID       string
	Repo     string
	Revision string
	License  string
	Files    []File
	Onnx     string
	// Output selects the model output; empty means the first one, mean-pooled by hugot.
	Output      string
	QueryPrefix string
	DocPrefix   string
	Dims        int
	// Truncate keeps the first N dimensions of a Matryoshka model; 0 keeps them all.
	Truncate int
	// MinScore is the similarity below which a match is noise for this model. Each model spreads
	// its cosines differently, so the threshold travels with the model.
	MinScore float32
	// NearScore is the similarity between two subjects above which they talk about the same thing.
	NearScore float32
}

// Default is the model `rtk models pull` fetches and the server looks for.
const Default = "multilingual-e5-small"

var Models = map[string]Spec{
	"multilingual-e5-small": {
		ID: "multilingual-e5-small", Repo: "Xenova/multilingual-e5-small", Revision: "761b726dd34fb83930e26aab4e9ac3899aa1fa78",
		License: "MIT",
		Files: []File{
			{"config.json", "cb99455288675345e1a4f411438d5d0adbba5fbd3a67ea4fb03c015433b996c1", 658},
			{"tokenizer.json", "0b44a9d7b51c3c62626640cda0e2c2f70fdacdc25bbbd68038369d14ebdf4c39", 17082730},
			{"tokenizer_config.json", "a1d6bc8734a6f635dc158508bef000f8e2e5a759c7d92f984b2c86e5ff53425b", 443},
			{"special_tokens_map.json", "d05497f1da52c5e09554c0cd874037a083e1dc1b9cfd48034d1c717f1afc07a7", 167},
			{"onnx/model_quantized.onnx", "f80102d3f2a1229f387d3c81909990d8945513e347b0eab049f7de3c6f98c193", 118308185},
		},
		Onnx: "model_quantized.onnx", QueryPrefix: "query: ", DocPrefix: "passage: ", Dims: 384, MinScore: 0.81, NearScore: 0.90,
	},
	"embeddinggemma-300m": {
		ID: "embeddinggemma-300m", Repo: "onnx-community/embeddinggemma-300m-ONNX", Revision: "5090578d9565bb06545b4552f76e6bc2c93e4a66",
		License: "Gemma Terms of Use",
		Files: []File{
			{"config.json", "6e1f06404b7163e0325ed2ea3e6781cde50f4a50b31780a95ad0d30e8404d77b", 1765},
			{"tokenizer.json", "4dda02faaf32bc91031dc8c88457ac272b00c1016cc679757d1c441b248b9c47", 20323312},
			{"tokenizer_config.json", "3ca953eea6c3c9fcda9cf3df22949ff18b216f7c74bd6459230f3f1013953f3a", 1156830},
			{"special_tokens_map.json", "2f7b0adf4fb469770bb1490e3e35df87b1dc578246c5e7e6fc76ecf33213a397", 662},
			{"onnx/model.onnx", "ea91fd315a7c152d427d231746f0f811a1ac93beaba656abfdf2b24e091265e4", 479932},
			{"onnx/model.onnx_data", "ef835ae565d8695236652475903078e8ed794c7c35faf1164d78ec3238e8a88d", 1234521088},
		},
		Onnx: "model.onnx", Output: "sentence_embedding",
		QueryPrefix: "task: search result | query: ", DocPrefix: "title: none | text: ",
		Dims: 256, Truncate: 256, MinScore: 0.34, NearScore: 0.65,
	},
}

func Lookup(id string) (Spec, error) {
	s, ok := Models[id]
	if !ok {
		ids := make([]string, 0, len(Models))
		for k := range Models {
			ids = append(ids, k)
		}
		sort.Strings(ids)
		return Spec{}, fmt.Errorf("embed: unknown model %q, known: %v", id, ids)
	}
	return s, nil
}

// Size is the download size of the model, in bytes.
func (s Spec) Size() int64 {
	var n int64
	for _, f := range s.Files {
		n += f.Size
	}
	return n
}

// ModelsDir is where pulled models live: $RTK_MODELS, else the user cache. Models are never in git.
func ModelsDir() (string, error) {
	if d := os.Getenv("RTK_MODELS"); d != "" {
		return d, nil
	}
	c, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("embed: no cache dir, set RTK_MODELS: %w", err)
	}
	return filepath.Join(c, "rtk", "models"), nil
}

func (s Spec) Dir() (string, error) {
	d, err := ModelsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, s.ID), nil
}

// hubURL is a variable only so tests can serve the files locally.
var hubURL = "https://huggingface.co"

var ErrMissing = errors.New("embed: model not downloaded, run `rtk models pull`")

// Present reports whether every file is on disk with the expected size. Hashes are checked once,
// at download, not on every start.
func (s Spec) Present() error {
	dir, err := s.Dir()
	if err != nil {
		return err
	}
	for _, f := range s.Files {
		st, err := os.Stat(filepath.Join(dir, path.Base(f.Path)))
		if err != nil || st.Size() != f.Size {
			return fmt.Errorf("%w (%s)", ErrMissing, s.ID)
		}
	}
	return nil
}

// Pull downloads the pinned files that are missing and verifies their hashes. The layout is flat
// because the runtime expects the tokenizer next to the ONNX file.
func (s Spec) Pull(ctx context.Context, progress func(file string, done, total int64)) error {
	dir, err := s.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	for _, f := range s.Files {
		dst := filepath.Join(dir, path.Base(f.Path))
		if st, err := os.Stat(dst); err == nil && st.Size() == f.Size {
			continue
		}
		url := fmt.Sprintf("%s/%s/resolve/%s/%s", hubURL, s.Repo, s.Revision, f.Path)
		if err := download(ctx, url, dst, f, progress); err != nil {
			return err
		}
	}
	return nil
}

func download(ctx context.Context, url, dst string, f File, progress func(string, int64, int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("embed: download %s: %w", f.Path, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("embed: download %s: %s", f.Path, res.Status)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".pull-*")
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	w := io.MultiWriter(tmp, h)
	var done int64
	buf := make([]byte, 1<<20)
	body := io.LimitReader(res.Body, f.Size+1)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, err := w.Write(buf[:n]); err != nil {
				tmp.Close()
				return fmt.Errorf("embed: write %s: %w", f.Path, err)
			}
			done += int64(n)
			if progress != nil {
				progress(f.Path, done, f.Size)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			tmp.Close()
			return fmt.Errorf("embed: download %s: %w", f.Path, rerr)
		}
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); done != f.Size || got != f.SHA256 {
		return fmt.Errorf("embed: %s does not match its pinned hash (got %d bytes, sha256 %s)", f.Path, done, got)
	}
	return os.Rename(tmp.Name(), dst)
}
