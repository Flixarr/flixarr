package store

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/secret"
)

func TestIssues(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	admin, _ := st.CreateUser(User{PlexID: 1, Username: "admin", Role: "Admin"})
	eryn, _ := st.CreateUser(User{PlexID: 2, Username: "eryn", Role: "User"})
	is, err := st.CreateIssue(Issue{Type: "series", TmdbID: 1396, Title: "Breaking Bad", Kind: "Audio", Sub: "Too quiet", Season: 2, Episode: 3, Note: "quiet", UserID: eryn.ID})
	if err != nil || is.Status != "open" || is.UserName != "eryn" {
		t.Fatalf("create: %+v %v", is, err)
	}
	st.AddComment(is.ID, admin.ID, "looking") // (returns the comment id)
	if cs, _ := st.Comments(is.ID); len(cs) != 1 || cs[0].UserName != "admin" {
		t.Fatalf("comments: %+v", cs)
	}
	st.SetIssueStatus(is.ID, admin.ID, "resolved")
	if open, _ := st.Issues(0, "open"); len(open) != 0 {
		t.Fatalf("open after resolve: %d", len(open))
	}
	if mine, _ := st.Issues(eryn.ID, ""); len(mine) != 1 || mine[0].Comments != 1 {
		t.Fatalf("eryn's: %+v", mine)
	}
	if theirs, _ := st.Issues(admin.ID, ""); len(theirs) != 0 {
		t.Fatalf("admin's own: %d", len(theirs))
	}
	st.DeleteUser(eryn.ID) // their issues go with them
	if all, _ := st.Issues(0, ""); len(all) != 0 {
		t.Fatalf("after removing the reporter: %d", len(all))
	}
}

func TestIssueBellSig(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := Open(dir, box)
	t.Cleanup(func() { st.Close() })
	u, _ := st.CreateUser(User{PlexID: 1, Username: "admin", Role: "Admin"})
	is, _ := st.CreateIssue(Issue{Type: "movie", TmdbID: 603, Title: "The Matrix", Kind: "Video", UserID: u.ID})
	n, sig := st.OpenIssueSig()
	if n != 1 || sig != "i"+strconv.FormatInt(is.ID, 10) {
		t.Fatalf("one issue: %d %q", n, sig)
	}
	st.RaiseAlert(Alert{Key: "issues:open", Audience: "can:manageIssues", Title: "1 open issue", Sig: sig})
	st.MarkAlertSeen(u.ID, "issues:open")
	time.Sleep(1100 * time.Millisecond)
	cid, _ := st.AddComment(is.ID, u.ID, "still broken")
	_, sig2 := st.OpenIssueSig()
	if !strings.Contains(sig2, "c"+strconv.FormatInt(cid, 10)) || !Louder(sig, sig2) {
		t.Fatalf("a reply makes it louder: %q -> %q", sig, sig2)
	}
	st.RaiseAlert(Alert{Key: "issues:open", Audience: "can:manageIssues", Title: "1 open issue", Sig: sig2})
	if l, _ := st.ActiveAlerts(9, CanAll); len(l) != 1 || !l[0].Unread {
		t.Fatalf("another manager sees it unread: %+v", l)
	}
}
