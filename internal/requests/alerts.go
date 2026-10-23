package requests

import (
	"fmt"

	"github.com/Flixarr/flixarr/internal/store"
)

// Alerts (the bell, #90) for requests: one alert per thing that happens. The person who asked hears
// Ready to watch, Couldn't get it, Removed and Left Plex (and Approved / Declined from Decided); people who
// manage requests get a to-do for each request waiting for approval and each one that failed, gone once
// it's handled. The steps in between (sent, downloading…) never alert.

func approveKey(id int64) string { return fmt.Sprintf("approve:req:%d", id) }
func failedKey(id int64) string  { return fmt.Sprintf("failed:req:%d", id) }
func readyKey(id int64) string   { return fmt.Sprintf("ready:req:%d", id) }

// Alert updates the bell for a request whose status just changed.
func (s *Service) Alert(r store.Request) {
	if r.Status != "pending" {
		s.Store.ResolveTodo(approveKey(r.ID))
	}
	if r.Status != "failed" {
		s.Store.ResolveTodo(failedKey(r.ID))
	}
	mine := fmt.Sprintf("user:%d", r.UserID)
	link := fmt.Sprintf("/%s/%d", r.Type, r.TmdbID)
	switch r.Status {
	case "pending":
		s.Store.RaiseTodo(store.Alert{Key: approveKey(r.ID), Audience: "can:manageRequests", Title: who(r) + " requested", Detail: r.Title + " · waiting for approval",
			Link: fmt.Sprintf("/requests?open=%d", r.ID), Poster: r.Poster})
		s.Store.TapAlertKey(r.UserID, approveKey(r.ID)) // they asked; they know
	case "failed":
		s.Store.AddNews(store.Alert{Audience: mine, Title: "Couldn't get it", Detail: r.Title, Link: fmt.Sprintf("/requests?open=%d", r.ID), Poster: r.Poster})
		s.Store.RaiseTodo(store.Alert{Key: failedKey(r.ID), Audience: "can:manageRequests", Title: "Request failed", Detail: r.Title + " · " + r.Error,
			Link: fmt.Sprintf("/requests?open=%d", r.ID), Poster: r.Poster})
	case "available":
		title := "Ready to watch"
		if prev, ok := s.Store.LastAlertAt(readyKey(r.ID)); ok {
			// Ready before: fixed or redownloaded since, or (a whole show) new episodes came in.
			if r.Type == "series" && !s.Store.TitleEventSince(r.Type, r.TmdbID, "redownload", prev) {
				title = "New episodes are ready to watch"
			} else {
				title = "Ready to watch again"
			}
		}
		s.Store.AddNews(store.Alert{Key: readyKey(r.ID), Audience: mine, Title: title, Detail: r.Title, Link: link, Poster: r.Poster, Repeat: true})
	case "removed":
		s.Store.AddNews(store.Alert{Audience: mine, Title: "Removed", Detail: r.Title + " · request it again to get it back", Link: link, Poster: r.Poster})
	case "expired":
		s.Store.AddNews(store.Alert{Audience: mine, Title: "Left Plex", Detail: r.Title + " · request it again to get it back", Link: link, Poster: r.Poster})
	}
}

// Decided tells the person who asked that someone else approved or declined their request.
func (s *Service) Decided(r store.Request, by *store.User) {
	if by == nil || by.ID == r.UserID {
		return
	}
	title, detail := "Approved", r.Title+" · by "+by.Username
	if r.Status == "declined" {
		title, detail = "Declined", r.Title
		if r.Error != "" {
			detail += " · " + r.Error
		}
	}
	s.Store.AddNews(store.Alert{Audience: fmt.Sprintf("user:%d", r.UserID), Title: title, Detail: detail, Link: fmt.Sprintf("/requests?open=%d", r.ID), Poster: r.Poster})
}

// Forget clears a removed request's to-dos.
func (s *Service) Forget(r store.Request) {
	s.Store.ResolveTodo(approveKey(r.ID))
	s.Store.ResolveTodo(failedKey(r.ID))
}

func who(r store.Request) string {
	if r.UserName != "" {
		return r.UserName
	}
	return "Someone"
}
