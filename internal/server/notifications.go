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
	notify.Channel
	WebhookURLHint string `json:"webhookUrlHint,omitempty"`
	URLHint        string `json:"urlHint,omitempty"`
	AuthSet        bool   `json:"authSet"`
	PasswordSet    bool   `json:"passwordSet"`
	TokenSet       bool   `json:"tokenSet"`
}

func hint(s string) string {
	if len(s) <= 6 {
		return ""
	}
	return "…" + s[len(s)-6:]
}

func viewOf(c notify.Channel) channelView {
	v := channelView{Channel: c, WebhookURLHint: hint(c.WebhookURL), AuthSet: c.Auth != ""}
	if u, err := url.Parse(c.URL); err == nil && u.Host != "" {
		v.URLHint = u.Scheme + "://" + u.Host + "/…" // the path can hold a token, so only the host is shown
	}
	v.PasswordSet, v.TokenSet = c.Password != "", c.Token != ""
	v.WebhookURL, v.Auth, v.Password, v.Token = "", "", "", ""
	return v
}

func (s *Server) notifyList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	out := []channelView{}
	for _, c := range s.Notify.Channels() {
		out = append(out, viewOf(c))
	}
	defs := map[string]channelView{}
	for _, svc := range notify.Services {
		defs[svc] = viewOf(notify.Channel{Service: svc, Name: notify.Labels[svc], Config: notify.Defaults(svc)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": out, "defaults": defs, "events": s.withoutMaintainerr(notify.Events), "appUrl": s.Store.Setting("app_url", "")})
}

// withoutMaintainerr drops the Leaving soon event until Maintainerr is connected (it can't happen).
func (s *Server) withoutMaintainerr(events []string) []string {
	if s.maintainerrOn() {
		return events
	}
	return slices.DeleteFunc(slices.Clone(events), func(e string) bool { return e == "request_leaving" })
}

// merged reads a channel form: a saved channel's ID (empty for a new one), its service, name and
// settings. Secrets left empty keep the saved value.
func (s *Server) merged(w http.ResponseWriter, r *http.Request, id string) (notify.Channel, bool) {
	var in struct {
		notify.Channel
		ClearAuth bool `json:"clearAuth"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return notify.Channel{}, false
	}
	c := in.Channel
	if id == "" {
		id = c.ID
	}
	old := notify.Channel{Service: c.Service, Config: notify.Defaults(c.Service)}
	if id != "" {
		saved, ok := s.Notify.Get(id)
		if !ok {
			writeErr(w, http.StatusNotFound, "That channel is gone.")
			return notify.Channel{}, false
		}
		old = saved
	}
	c.ID, c.Service = id, old.Service
	if !slices.Contains(notify.Services, c.Service) {
		writeErr(w, http.StatusNotFound, "Unknown service.")
		return notify.Channel{}, false
	}
	if c.WebhookURL == "" {
		c.WebhookURL = old.WebhookURL
	}
	if c.URL == "" && c.Service == "webhook" {
		c.URL = old.URL
	}
	if c.Auth == "" && !in.ClearAuth {
		c.Auth = old.Auth
	}
	if c.Password == "" && !in.ClearAuth {
		c.Password = old.Password
	}
	if c.Token == "" && !in.ClearAuth {
		c.Token = old.Token
	}
	if c.Events == nil {
		c.Events = []string{}
	}
	return c, true
}

func (s *Server) notifyAdd(w http.ResponseWriter, r *http.Request, _ *store.User) {
	c, ok := s.merged(w, r, "")
	if !ok {
		return
	}
	c.ID = ""
	c, err := s.Notify.Add(c)
	if err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, viewOf(c))
}

func (s *Server) notifySave(w http.ResponseWriter, r *http.Request, _ *store.User) {
	c, ok := s.merged(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.Notify.Update(c); err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	saved, _ := s.Notify.Get(c.ID)
	writeJSON(w, http.StatusOK, viewOf(saved))
}

func (s *Server) notifyDelete(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if err := s.Notify.Delete(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, capitalize(err.Error())+".")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// notifyTest sends a test through a channel's settings as they are in the form (saved or not).
func (s *Server) notifyTest(w http.ResponseWriter, r *http.Request, u *store.User) {
	c, ok := s.merged(w, r, "")
	if !ok {
		return
	}
	c.TestTo, c.TestUser = u.Email, u.ID // the test goes to whoever pressed it
	if err := s.Notify.Test(r.Context(), c.Service, c.Config); err != nil {
		writeErr(w, http.StatusBadGateway, "Test not sent: "+err.Error()+".")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
