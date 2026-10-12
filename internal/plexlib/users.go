package plexlib

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// Friend is a Plex account the connected server is shared with.
type Friend struct {
	PlexID   int64  `json:"plexId"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Thumb    string `json:"avatar"`
}

func (l *Library) tvBase() string {
	if l.TVBaseURL != "" {
		return l.TVBaseURL
	}
	return "https://plex.tv"
}

// Friends lists the Plex accounts the connected server is shared with (plex.tv's friends list,
// which is only available as XML). Plex Home managed users aren't included: they have no plex.tv
// sign-in of their own.
func (l *Library) Friends(ctx context.Context) ([]Friend, error) {
	server := l.Store.Setting(keyServerID, "")
	if server == "" {
		return nil, errors.New("connect a Plex server first (Settings → Plex)")
	}
	tok := l.Store.SecretSetting(keyAdminToken, "")
	if tok == "" {
		return nil, errors.New("sign in with Plex again so Flixarr can see who your server is shared with")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.tvBase()+"/api/users", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Plex-Token", tok)
	req.Header.Set("X-Plex-Client-Identifier", l.ClientID)
	req.Header.Set("X-Plex-Product", "Flixarr")
	res, err := l.client().Do(req)
	if err != nil {
		return nil, errors.New("couldn't reach plex.tv")
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("plex.tv answered %s", res.Status)
	}
	var doc struct {
		Users []struct {
			ID       string `xml:"id,attr"`
			Title    string `xml:"title,attr"`
			Username string `xml:"username,attr"`
			Email    string `xml:"email,attr"`
			Thumb    string `xml:"thumb,attr"`
			Servers  []struct {
				Machine string `xml:"machineIdentifier,attr"`
				Pending string `xml:"pending,attr"`
			} `xml:"Server"`
		} `xml:"User"`
	}
	if err := xml.NewDecoder(res.Body).Decode(&doc); err != nil {
		return nil, errors.New("plex.tv sent something unexpected")
	}
	out := []Friend{}
	for _, u := range doc.Users {
		id, _ := strconv.ParseInt(u.ID, 10, 64)
		shared := false
		for _, s := range u.Servers {
			shared = shared || (s.Machine == server && s.Pending != "1")
		}
		if id == 0 || !shared {
			continue
		}
		name := u.Username
		if name == "" {
			name = u.Title
		}
		out = append(out, Friend{PlexID: id, Username: name, Email: u.Email, Thumb: u.Thumb})
	}
	return out, nil
}

// HasAccess reports whether the account behind a plex.tv token can use the connected server.
func (l *Library) HasAccess(ctx context.Context, token string) (bool, error) {
	server := l.Store.Setting(keyServerID, "")
	if server == "" {
		return false, nil
	}
	var res []struct {
		ClientIdentifier string `json:"clientIdentifier"`
	}
	if err := l.get(ctx, l.tvBase(), "/api/v2/resources", token, &res); err != nil {
		return false, err
	}
	for _, r := range res {
		if r.ClientIdentifier == server {
			return true, nil
		}
	}
	return false, nil
}
