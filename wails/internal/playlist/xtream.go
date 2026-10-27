package playlist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// Xtream is an Xtream Codes account.
type Xtream struct {
	Server   string
	Username string
	Password string
}

type xtreamCategory struct {
	ID   json.RawMessage `json:"category_id"`
	Name string          `json:"category_name"`
}

type xtreamStream struct {
	Name       string          `json:"name"`
	StreamID   json.RawMessage `json:"stream_id"`
	Icon       string          `json:"stream_icon"`
	CategoryID json.RawMessage `json:"category_id"`
}

// LiveChannels lists the account's live TV streams.
func (x Xtream) LiveChannels(ctx context.Context) ([]Entry, error) {
	base, err := x.base()
	if err != nil {
		return nil, err
	}

	groups := map[string]string{}
	err = xtreamList(ctx, x, base, "get_live_categories", func(c xtreamCategory) {
		groups[rawID(c.ID)] = c.Name
	})
	if err != nil {
		return nil, err
	}

	// Streams become entries as they are decoded: holding the whole list
	// first is what let a body of `[{},{},…]` take gigabytes.
	user, pass := url.PathEscape(x.Username), url.PathEscape(x.Password)
	var entries []Entry
	err = xtreamList(ctx, x, base, "get_live_streams", func(s xtreamStream) {
		if id := rawID(s.StreamID); id != "" {
			entries = append(entries, Entry{
				Name:  strings.TrimSpace(s.Name),
				Group: groups[rawID(s.CategoryID)],
				Logo:  logo(s.Icon),
				URL:   base + "/live/" + user + "/" + pass + "/" + url.PathEscape(id) + ".ts",
			})
		}
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// Location is the account as shown in the UI, without credentials.
func (x Xtream) Location() string {
	return strings.TrimRight(strings.TrimSpace(x.Server), "/")
}

// base validates the server and returns it exactly as typed (trimmed), not
// re-serialised: Go and Rust normalise URLs differently, and the stream URL
// feeds the channel ID, so both builds must start from the same string.
func (x Xtream) base() (string, error) {
	loc := x.Location()
	u, err := ValidateHTTPURL(loc)
	if err != nil {
		return "", err
	}
	if u.User != nil {
		return "", errors.New("credentials_in_url")
	}
	return loc, nil
}

// xtreamList streams the JSON array an API action returns, handing each
// element to fn.
func xtreamList[T any](ctx context.Context, x Xtream, base, action string, fn func(T)) error {
	q := url.Values{"username": {x.Username}, "password": {x.Password}, "action": {action}}
	b, err := open(ctx, base+"/player_api.php?"+q.Encode(), maxAPIBytes)
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	return decodeList(b, fn)
}

// decodeList stops after MaxEntries elements, truncating like ParseM3U does
// rather than failing the import; the rest of the body is never read.
func decodeList[T any](b *body, fn func(T)) error {
	dec := json.NewDecoder(b)
	// Panels answer bad credentials with an object (or nothing) instead of a list.
	if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
		if rerr := b.readErr(); rerr != nil {
			return rerr
		}
		return errors.New("xtream_rejected")
	}
	for range MaxEntries {
		if !dec.More() {
			break
		}
		var item T
		if err := dec.Decode(&item); err != nil {
			return invalidResponse(b)
		}
		fn(item)
	}
	if dec.More() {
		return nil
	}
	if _, err := dec.Token(); err != nil { // the closing ]
		return invalidResponse(b)
	}
	return nil
}

func invalidResponse(b *body) error {
	if err := b.readErr(); err != nil {
		return err
	}
	return errors.New("xtream_invalid_response")
}

func logo(raw string) string {
	if isLogoURL(raw) {
		return raw
	}
	return ""
}

// rawID normalises IDs that panels send either as numbers or as strings;
// anything else is no ID. Matches raw_id in playlist.rs.
func rawID(raw json.RawMessage) string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return ""
	}
	switch v := v.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

// Account is the subscription info an Xtream panel reports.
type Account struct {
	// ExpiresAt is unix seconds; 0 means unlimited or unknown.
	ExpiresAt int64
	// Status as the panel words it, e.g. "Active", "Expired", "Banned".
	Status string
}

// AccountInfo asks the panel for the account's expiry and status (the
// player API without an action).
func (x Xtream) AccountInfo(ctx context.Context) (Account, error) {
	base, err := x.base()
	if err != nil {
		return Account{}, err
	}
	q := url.Values{"username": {x.Username}, "password": {x.Password}}
	// The answer is a small object; a megabyte is plenty.
	b, err := open(ctx, base+"/player_api.php?"+q.Encode(), 1<<20)
	if err != nil {
		return Account{}, err
	}
	defer func() { _ = b.Close() }()
	var resp struct {
		UserInfo struct {
			ExpDate json.RawMessage `json:"exp_date"`
			Status  string          `json:"status"`
		} `json:"user_info"`
	}
	if err := json.NewDecoder(b).Decode(&resp); err != nil {
		return Account{}, invalidResponse(b)
	}
	exp, _ := strconv.ParseInt(rawID(resp.UserInfo.ExpDate), 10, 64)
	return Account{ExpiresAt: max(exp, 0), Status: strings.TrimSpace(resp.UserInfo.Status)}, nil
}
