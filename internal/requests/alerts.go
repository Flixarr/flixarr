package requests

import (
	"fmt"
	"strings"

	"github.com/Flixarr/flixarr/internal/store"
)

// Bell alerts for requests, grouped (Rule 19): one per person about their own requests, updated in place
// as they move along, and one for admins while requests wait for approval. Never one per download.

func userAlertKey(userID int64) string { return fmt.Sprintf("requests:user:%d", userID) }

var alertText = map[string]string{
	"approved": "%s was approved", "sent": "%s was sent to be downloaded", "grabbed": "%s was found and is about to download",
	"downloading": "%s is downloading", "importing": "%s is being added to Plex", "available": "%s is ready to watch",
	"failed": "%s couldn't be added", "declined": "%s was declined",
}

// Alert updates the requester's bell for a request that just changed status, and the admins' waiting count.
func (s *Service) Alert(r store.Request) {
	if f, ok := alertText[r.Status]; ok {
		detail := fmt.Sprintf(f, r.Title)
		if r.Status == "declined" && r.Error != "" {
			detail += ": " + r.Error
		}
		if r.Status == "failed" && strings.HasPrefix(r.Error, removedNote+" from Plex") {
			detail = r.Title + " was removed from Plex"
		}
		if n := s.activeCount(r.UserID); n > 0 && r.Status != "downloading" && r.Status != "grabbed" && r.Status != "importing" && r.Status != "sent" && r.Status != "approved" {
			detail += fmt.Sprintf(" · %d more on the way", n)
		}
		sev := "info"
		if r.Status == "failed" || r.Status == "declined" {
			sev = "warning"
		}
		s.Store.RaiseAlert(store.Alert{Key: userAlertKey(r.UserID), Audience: fmt.Sprintf("user:%d", r.UserID), Severity: sev,
			Title: "Your requests", Detail: detail, Link: "/requests", Sig: fmt.Sprintf("%d:%s", r.ID, alertStep(r.Status))})
	}
	s.alertWaiting()
}

// alertStep groups the in-between statuses: moving along (grabbed, downloading, importing) updates the
// bell's text without turning it red; approved, ready, failed and declined do.
func alertStep(status string) string {
	switch status {
	case "sent", "grabbed", "downloading", "importing":
		return "moving"
	}
	return status
}

func (s *Service) activeCount(userID int64) int {
	list, _ := s.Store.Requests(userID, 200)
	n := 0
	for _, q := range list {
		switch q.Status {
		case "approved", "sent", "grabbed", "downloading", "importing":
			n++
		}
	}
	return n
}

// alertWaiting keeps the admins' "N requests waiting for approval" alert current.
func (s *Service) alertWaiting() {
	list, _ := s.Store.Requests(0, 1000)
	n := 0
	for _, q := range list {
		if q.Status == "pending" {
			n++
		}
	}
	if n == 0 {
		s.Store.ResolveAlert("requests:pending")
		return
	}
	title := "1 request waiting"
	if n > 1 {
		title = fmt.Sprintf("%d requests waiting", n)
	}
	s.Store.RaiseAlert(store.Alert{Key: "requests:pending", Audience: "admins", Severity: "info", Title: title,
		Detail: "Approve or decline on the Requests page", Link: "/requests", Sig: fmt.Sprint(n)})
}

// SeenAlerts clears a person's request alert when they open the bell again after already seeing it,
// and nothing of theirs is still on its way.
func (s *Service) SeenAlerts(userID int64) {
	list, _ := s.Store.ActiveAlerts(userID, false)
	for _, a := range list {
		if a.Key == userAlertKey(userID) && !a.Unread && s.activeCount(userID) == 0 {
			s.Store.ResolveAlert(a.Key)
		}
	}
}

// Forgotten updates the waiting count after a request is removed.
func (s *Service) Forgotten() { s.alertWaiting() }
