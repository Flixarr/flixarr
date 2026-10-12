package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"

	"github.com/Flixarr/flixarr/internal/notify"
	"github.com/Flixarr/flixarr/internal/store"
)

// channelView is a channel as Settings shows it: secrets replaced by a hint of their last characters.
type channelView struct {
	notify.Config
	WebhookURLHint string `json:"webhookUrlHint,omitempty"`
	URLHint        string `json:"urlHint,omitempty"`
	AuthSet        bool   `json:"authSet"`
	PasswordSet    bool   `json:"passwordSet"`
}

func hint(s string) string {
	if len(s) <= 6 {
		return ""
	}
	return "…" + s[len(s)-6:]
}

func viewOf(c notify.Config) channelView {
	v := channelView{Config: c, WebhookURLHint: hint(c.WebhookURL), AuthSet: c.Auth != ""}
	if u, err := url.Parse(c.URL); err == nil && u.Host != "" {
		v.URLHint = u.Scheme + "://" + u.Host + "/…" // the path can hold a token, so only the host is shown
	}
	v.PasswordSet = c.Password != ""
	v.WebhookURL, v.Auth, v.Password = "", "", ""
	return v
}

func (s *Server) notifyList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	out := map[string]channelView{}
	for _, ch := range notify.Channels {
		out[ch] = viewOf(s.Notify.Load(ch))
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": out, "events": notify.Events, "appUrl": s.Store.Setting("app_url", "")})
}

// merged reads a channel form; secrets left empty keep the saved value.
func (s *Server) merged(w http.ResponseWriter, r *http.Request) (string, notify.Config, bool) {
	ch := r.PathValue("channel")
	if !slices.Contains(notify.Channels, ch) {
		writeErr(w, http.StatusNotFound, "Unknown channel.")
		return "", notify.Config{}, false
	}
	var in struct {
		notify.Config
		ClearAuth bool `json:"clearAuth"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return "", notify.Config{}, false
	}
	old := s.Notify.Load(ch)
	c := in.Config
	if c.WebhookURL == "" {
		c.WebhookURL = old.WebhookURL
	}
	if c.URL == "" && ch == "webhook" {
		c.URL = old.URL
	}
	if c.Auth == "" && !in.ClearAuth {
		c.Auth = old.Auth
	}
	if c.Password == "" && !in.ClearAuth {
		c.Password = old.Password
	}
	if c.Events == nil {
		c.Events = []string{}
	}
	return ch, c, true
}

func (s *Server) notifySave(w http.ResponseWriter, r *http.Request, _ *store.User) {
	ch, c, ok := s.merged(w, r)
	if !ok {
		return
	}
	c.TestTo = ""
	if err := s.Notify.Save(ch, c); err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, viewOf(s.Notify.Load(ch)))
}

func (s *Server) notifyTest(w http.ResponseWriter, r *http.Request, u *store.User) {
	ch, c, ok := s.merged(w, r)
	if !ok {
		return
	}
	c.TestTo = u.Email // the email test goes to whoever pressed it
	if err := s.Notify.Test(r.Context(), ch, c); err != nil {
		writeErr(w, http.StatusBadGateway, "Test not sent: "+err.Error()+".")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
