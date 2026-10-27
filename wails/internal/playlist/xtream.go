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

	var cats []xtreamCategory
	if err := x.call(ctx, base, "get_live_categories", &cats); err != nil {
		return nil, err
	}
	groups := make(map[string]string, len(cats))
	for _, c := range cats {
		groups[rawID(c.ID)] = c.Name
	}

	var streams []xtreamStream
	if err := x.call(ctx, base, "get_live_streams", &streams); err != nil {
		return nil, err
	}

	user, pass := url.PathEscape(x.Username), url.PathEscape(x.Password)
	entries := make([]Entry, 0, min(len(streams), MaxEntries))
	for _, s := range streams {
		if len(entries) == MaxEntries {
			break
		}
		id := rawID(s.StreamID)
		if id == "" {
			continue
		}
		entries = append(entries, Entry{
			Name:  strings.TrimSpace(s.Name),
			Group: groups[rawID(s.CategoryID)],
			Logo:  logo(s.Icon),
			URL:   base + "/live/" + user + "/" + pass + "/" + url.PathEscape(id) + ".ts",
		})
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

func (x Xtream) call(ctx context.Context, base, action string, out any) error {
	q := url.Values{"username": {x.Username}, "password": {x.Password}, "action": {action}}
	body, err := get(ctx, base+"/player_api.php?"+q.Encode())
	if err != nil {
		return err
	}
	// Panels answer bad credentials with an object (or nothing) instead of a list.
	if !bytes.HasPrefix(bytes.TrimSpace(body), []byte("[")) {
		return errors.New("xtream_rejected")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return errors.New("xtream_invalid_response")
	}
	return nil
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
	body, err := get(ctx, base+"/player_api.php?"+q.Encode())
	if err != nil {
		return Account{}, err
	}
	var resp struct {
		UserInfo struct {
			ExpDate json.RawMessage `json:"exp_date"`
			Status  string          `json:"status"`
		} `json:"user_info"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return Account{}, errors.New("xtream_invalid_response")
	}
	exp, _ := strconv.ParseInt(rawID(resp.UserInfo.ExpDate), 10, 64)
	return Account{ExpiresAt: max(exp, 0), Status: strings.TrimSpace(resp.UserInfo.Status)}, nil
}
