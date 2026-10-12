package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Flixarr/flixarr/internal/store"
)

var (
	reRegion   = regexp.MustCompile(`^[A-Z]{2}$`)
	reLanguage = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)
)

type generalSettings struct {
	Region   string `json:"region"`   // ISO 3166-1, for release dates, age ratings and "upcoming"
	Language string `json:"language"` // TMDB language for titles and overviews, e.g. en-US
	AppURL   string `json:"appUrl"`   // where people reach Flixarr; used in links that leave the app
}

func (s *Server) generalGet() generalSettings {
	return generalSettings{Region: s.Store.Setting("region", "US"), Language: s.Store.Setting("language", "en-US"), AppURL: s.Store.Setting("app_url", "")}
}

// generalSettings reads or changes Settings → General's region, language and application URL.
func (s *Server) generalSettings(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if r.Method == http.MethodPut {
		var in generalSettings
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "Bad request.")
			return
		}
		if !reRegion.MatchString(in.Region) || !reLanguage.MatchString(in.Language) {
			writeErr(w, http.StatusBadRequest, "Pick a region and language from the lists.")
			return
		}
		in.AppURL = strings.TrimRight(strings.TrimSpace(in.AppURL), "/")
		if in.AppURL != "" {
			u, err := url.Parse(in.AppURL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				writeErr(w, http.StatusBadRequest, "The application URL should look like https://flixarr.example.com.")
				return
			}
		}
		s.Store.SetSetting("region", in.Region)
		s.Store.SetSetting("language", in.Language)
		s.Store.SetSetting("app_url", in.AppURL)
	}
	writeJSON(w, http.StatusOK, s.generalGet())
}
