package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Flixarr/flixarr/internal/store"
)

// IssueKind is a reason someone can pick when reporting an issue ("What's wrong"), with its details.
type IssueKind struct {
	Kind string   `json:"kind"`
	Subs []string `json:"subs"`
}

// defaultIssueKinds: what can be wrong, in the order the report window shows them (Other has no details).
var defaultIssueKinds = []IssueKind{
	{"Video", []string{"Bad quality", "Wrong file", "Won't play"}},
	{"Audio", []string{"Too quiet", "Out of sync", "Wrong language", "No audio"}},
	{"Subtitles", []string{"Missing", "Out of sync", "Wrong language"}},
	// Not about a file: a request that never arrived. The one kind allowed on titles not in the library.
	{downloadKind, []string{"Never started", "Got stuck"}},
	{"Other", []string{}},
}

// downloadKind is the issue kind for a request that never arrived. Flixarr itself relies on it, so it can't be
// renamed or removed (its details can be changed).
const downloadKind = "Didn't download"

// keyIssueKinds holds an admin's own list (#194); none saved = the defaults.
const keyIssueKinds = "issue_kinds"

// issueKinds is the list people pick from: an admin's own, or the defaults.
func (s *Server) issueKinds() []IssueKind {
	var list []IssueKind
	if raw := s.Store.Setting(keyIssueKinds, ""); raw == "" || json.Unmarshal([]byte(raw), &list) != nil || len(list) == 0 {
		return defaultIssueKinds
	}
	return list
}

func issueKindMap(list []IssueKind) map[string][]string {
	out := map[string][]string{}
	for _, k := range list {
		out[k.Kind] = k.Subs
	}
	return out
}

// cleanIssueKinds checks an admin's list: names filled in and not too long, no repeats, Didn't download
// kept. It returns the list tidied up, or what's wrong in plain words.
func cleanIssueKinds(in []IssueKind) ([]IssueKind, string) {
	if len(in) > 20 {
		return nil, "Keep it to 20 reasons or fewer."
	}
	out := []IssueKind{}
	seen := map[string]bool{}
	hasDownload := false
	for _, k := range in {
		name := strings.TrimSpace(k.Kind)
		switch {
		case name == "":
			return nil, "Every reason needs a name."
		case len(name) > 40:
			return nil, "Keep reason names to 40 letters or fewer."
		case seen[strings.ToLower(name)]:
			return nil, "\"" + name + "\" is in the list twice."
		case len(k.Subs) > 20:
			return nil, "Keep it to 20 details per reason or fewer."
		}
		seen[strings.ToLower(name)] = true
		hasDownload = hasDownload || name == downloadKind
		subs := []string{}
		seenSub := map[string]bool{}
		for _, sub := range k.Subs {
			sub = strings.TrimSpace(sub)
			switch {
			case sub == "":
				return nil, "Every detail under " + name + " needs a name."
			case len(sub) > 40:
				return nil, "Keep details to 40 letters or fewer."
			case seenSub[strings.ToLower(sub)]:
				return nil, "\"" + sub + "\" is under " + name + " twice."
			}
			seenSub[strings.ToLower(sub)] = true
			subs = append(subs, sub)
		}
		out = append(out, IssueKind{name, subs})
	}
	if !hasDownload {
		return nil, downloadKind + " can't be removed: Flixarr uses it for requests that never arrived."
	}
	if len(out) < 2 {
		return nil, "Keep at least one reason besides " + downloadKind + "."
	}
	return out, ""
}

// issueKindsRead gives the list people pick from (the report window asks for it).
func (s *Server) issueKindsRead(w http.ResponseWriter, _ *http.Request, _ *store.User) {
	writeJSON(w, http.StatusOK, map[string]any{"kinds": s.issueKinds()})
}

// issueKindsSettings reads or changes Settings → Issues (#194): {kinds} saves an admin's own list, {reset}
// puts the defaults back. Issues already reported keep the reason they were filed with.
func (s *Server) issueKindsSettings(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if r.Method == http.MethodPut {
		var in struct {
			Kinds []IssueKind `json:"kinds"`
			Reset bool        `json:"reset"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "Bad request.")
			return
		}
		if in.Reset {
			s.Store.SetSetting(keyIssueKinds, "")
		} else {
			list, why := cleanIssueKinds(in.Kinds)
			if why != "" {
				writeErr(w, http.StatusBadRequest, why)
				return
			}
			raw, _ := json.Marshal(list)
			if err := s.Store.SetSetting(keyIssueKinds, string(raw)); err != nil {
				writeErr(w, http.StatusInternalServerError, "Database error.")
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"kinds": s.issueKinds(), "defaults": defaultIssueKinds, "custom": s.Store.Setting(keyIssueKinds, "") != ""})
}
