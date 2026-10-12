package store

import (
	"testing"

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
	st.AddComment(is.ID, admin.ID, "looking")
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
