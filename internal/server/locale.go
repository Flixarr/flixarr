package server

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/Flixarr/flixarr/internal/store"
)

// locale is the region and language for a request's lookups: the signed-in person's own, else the
// server's (Settings → General).
func (s *Server) locale(r *http.Request) (region, language string) {
	region, language = s.Store.Setting("region", "US"), s.Store.Setting("language", "en-US")
	if u, ok := r.Context().Value(ctxKey{}).(*store.User); ok && u != nil {
		if u.Region != "" {
			region = u.Region
		}
		if u.Language != "" {
			language = u.Language
		}
	}
	return region, language
}

var (
	regionRe   = regexp.MustCompile(`^[A-Z]{2}$`)
	languageRe = regexp.MustCompile(`^[a-z]{2}-[A-Z]{2}$`)
)

// myLocale reads or saves your own region and language (blank = the server's, which it also returns).
func (s *Server) myLocale(w http.ResponseWriter, r *http.Request, u *store.User) {
	if r.Method == http.MethodGet {
		s.localeOut(w, u)
		return
	}
	var in struct {
		Region   string `json:"region"`
		Language string `json:"language"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil ||
		(in.Region != "" && !regionRe.MatchString(in.Region)) || (in.Language != "" && !languageRe.MatchString(in.Language)) {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if err := s.Store.SetUserLocale(u.ID, in.Region, in.Language); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	u.Region, u.Language = in.Region, in.Language
	s.localeOut(w, u)
}

func (s *Server) localeOut(w http.ResponseWriter, u *store.User) {
	writeJSON(w, http.StatusOK, map[string]string{"region": u.Region, "language": u.Language,
		"serverRegion": s.Store.Setting("region", "US"), "serverLanguage": s.Store.Setting("language", "en-US")})
}
