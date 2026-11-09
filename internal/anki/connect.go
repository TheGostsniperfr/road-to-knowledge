package anki

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

const DefaultConnectURL = "http://127.0.0.1:8765"

var (
	ErrNotLocal       = errors.New("AnkiConnect must listen on localhost")
	ErrAnkiNotRunning = errors.New("Anki is not running, or the AnkiConnect add-on is not installed")
)

// Connect talks to the AnkiConnect add-on of a running Anki desktop.
type Connect struct {
	url  string
	http *http.Client
}

// NewConnect refuses anything but a loopback address: the push sends the whole deck, and
// AnkiConnect has no authentication by default.
func NewConnect(raw string) (*Connect, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" {
		return nil, fmt.Errorf("%w: %q", ErrNotLocal, raw)
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("%w: %q", ErrNotLocal, raw)
	}
	// A redirect could send the deck off the machine; AnkiConnect never redirects.
	noRedirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Connect{url: u.String(), http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: noRedirect}}, nil
}

func (c *Connect) call(ctx context.Context, action string, params, out any) error {
	body, _ := json.Marshal(map[string]any{"action": action, "version": 6, "params": params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ankiconnect %s: %w", action, err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		var op *net.OpError
		if errors.As(err, &op) || errors.Is(err, os.ErrDeadlineExceeded) {
			return fmt.Errorf("%w: %w", ErrAnkiNotRunning, err)
		}
		return fmt.Errorf("ankiconnect %s: %w", action, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return fmt.Errorf("ankiconnect %s: %w", action, err)
	}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *string         `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("ankiconnect %s: %w", action, err)
	}
	if env.Error != nil {
		return fmt.Errorf("ankiconnect %s: %s", action, *env.Error)
	}
	if out != nil {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("ankiconnect %s: %w", action, err)
		}
	}
	return nil
}

type PushStats struct{ Added, Updated, Unchanged int }

// Push adds the notes Anki does not have and updates the ones whose text changed, matching on
// the RtkID field. It never deletes: a card removed from the repo stays in Anki.
func (c *Connect) Push(ctx context.Context, notes []Note) (PushStats, error) {
	var st PushStats
	var version int
	if err := c.call(ctx, "version", nil, &version); err != nil {
		return st, err
	}
	var models []string
	if err := c.call(ctx, "modelNames", nil, &models); err != nil {
		return st, err
	}
	if !contains(models, ModelName) {
		if err := c.call(ctx, "createModel", map[string]any{
			"modelName": ModelName, "inOrderFields": Fields, "css": css, "isCloze": false,
			"cardTemplates": []map[string]string{{"Name": "Card 1", "Front": front, "Back": back}},
		}, nil); err != nil {
			return st, err
		}
	}
	var decks []string
	if err := c.call(ctx, "deckNames", nil, &decks); err != nil {
		return st, err
	}
	for _, d := range decksOf(notes) {
		if !contains(decks, d) {
			if err := c.call(ctx, "createDeck", map[string]string{"deck": d}, nil); err != nil {
				return st, err
			}
		}
	}

	var ids []int64
	if err := c.call(ctx, "findNotes", map[string]string{"query": `"note:` + ModelName + `"`}, &ids); err != nil {
		return st, err
	}
	type info struct {
		NoteID int64 `json:"noteId"`
		Fields map[string]struct {
			Value string `json:"value"`
		} `json:"fields"`
	}
	var infos []info
	if len(ids) > 0 {
		if err := c.call(ctx, "notesInfo", map[string]any{"notes": ids}, &infos); err != nil {
			return st, err
		}
	}
	existing := map[string]info{}
	for _, in := range infos {
		existing[in.Fields["RtkID"].Value] = in
	}

	var add []map[string]any
	var update []map[string]any
	for _, n := range notes {
		fields := map[string]string{"Front": html.EscapeString(n.Front), "Back": html.EscapeString(n.Back), "Subject": html.EscapeString(n.Subject), "RtkID": n.CardID}
		in, ok := existing[n.CardID]
		if !ok {
			add = append(add, map[string]any{"deckName": n.Deck, "modelName": ModelName, "fields": fields, "tags": n.Tags, "options": map[string]any{"allowDuplicate": true}})
			continue
		}
		if in.Fields["Front"].Value == fields["Front"] && in.Fields["Back"].Value == fields["Back"] && in.Fields["Subject"].Value == fields["Subject"] {
			st.Unchanged++
			continue
		}
		update = append(update, map[string]any{"action": "updateNoteFields", "params": map[string]any{"note": map[string]any{"id": in.NoteID, "fields": fields}}})
	}
	if len(update) > 0 {
		var results []json.RawMessage
		if err := c.call(ctx, "multi", map[string]any{"actions": update}, &results); err != nil {
			return st, err
		}
		st.Updated = len(update)
	}
	if len(add) > 0 {
		var added []*int64
		if err := c.call(ctx, "addNotes", map[string]any{"notes": add}, &added); err != nil {
			return st, err
		}
		for _, id := range added {
			if id != nil {
				st.Added++
			}
		}
	}
	return st, nil
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
