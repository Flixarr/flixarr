package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// Issue is a problem reported on a title in the library.
type Issue struct {
	ID         int64    `json:"id"`
	Type       string   `json:"type"`
	TmdbID     int      `json:"tmdbId"`
	Title      string   `json:"title"`
	Year       string   `json:"year"`
	Poster     string   `json:"poster"`
	Kind       string   `json:"kind"`
	Sub        string   `json:"sub"`
	Season     int      `json:"season"`
	Episode    int      `json:"episode"`
	Parts      [][2]int `json:"parts"` // series: [season, episode] pairs; episode 0 = the whole season
	Note       string   `json:"note"`
	UserID     int64    `json:"userId"`
	UserName   string   `json:"userName"`
	UserAvatar string   `json:"userAvatar"`
	Status     string   `json:"status"`
	Comments   int      `json:"comments"`
	CreatedAt  string   `json:"createdAt"`
	UpdatedAt  string   `json:"updatedAt"`
}

// Comment is one message on an issue.
type Comment struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"userId"`
	UserName   string `json:"userName"`
	UserAvatar string `json:"userAvatar"`
	Body       string `json:"body"`
	CreatedAt  string `json:"createdAt"`
}

const issueCols = `i.id, i.type, i.tmdb_id, i.title, i.year, i.poster, i.kind, i.sub, i.season, i.episode, i.note, i.user_id,
	COALESCE(u.username, ''), COALESCE(u.avatar, ''), i.status, (SELECT COUNT(*) FROM issue_comments c WHERE c.issue_id = i.id), i.created_at, i.updated_at, i.parts`

func scanIssue(row interface{ Scan(...any) error }) (Issue, error) {
	var i Issue
	var parts string
	err := row.Scan(&i.ID, &i.Type, &i.TmdbID, &i.Title, &i.Year, &i.Poster, &i.Kind, &i.Sub, &i.Season, &i.Episode, &i.Note, &i.UserID,
		&i.UserName, &i.UserAvatar, &i.Status, &i.Comments, &i.CreatedAt, &i.UpdatedAt, &parts)
	i.Parts = [][2]int{}
	json.Unmarshal([]byte(parts), &i.Parts)
	if errors.Is(err, sql.ErrNoRows) {
		return i, ErrNotFound
	}
	return i, err
}

// CreateIssue saves a new issue.
func (s *Store) CreateIssue(i Issue) (Issue, error) {
	t := now()
	if i.Parts == nil {
		i.Parts = [][2]int{}
	}
	parts, _ := json.Marshal(i.Parts)
	res, err := s.db.Exec(`INSERT INTO issues (type, tmdb_id, title, year, poster, kind, sub, season, episode, note, user_id, status, created_at, updated_at, parts)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', ?, ?, ?)`, i.Type, i.TmdbID, i.Title, i.Year, i.Poster, i.Kind, i.Sub, i.Season, i.Episode, i.Note, i.UserID, t, t, string(parts))
	if err != nil {
		return Issue{}, err
	}
	id, _ := res.LastInsertId()
	return s.Issue(id)
}

// Issue returns one issue.
func (s *Store) Issue(id int64) (Issue, error) {
	return scanIssue(s.db.QueryRow(`SELECT `+issueCols+` FROM issues i LEFT JOIN users u ON u.id = i.user_id WHERE i.id = ?`, id))
}

// Issues lists issues newest first; userID 0 = everyone's, status "" = any.
func (s *Store) Issues(userID int64, status string) ([]Issue, error) {
	rows, err := s.db.Query(`SELECT `+issueCols+` FROM issues i LEFT JOIN users u ON u.id = i.user_id
		WHERE (? = 0 OR i.user_id = ?) AND (? = '' OR i.status = ?) ORDER BY i.id DESC LIMIT 500`, userID, userID, status, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Issue{}
	for rows.Next() {
		i, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// SetIssueStatus resolves or reopens an issue.
func (s *Store) SetIssueStatus(id, by int64, status string) error {
	var resolver any
	if status == "resolved" {
		resolver = by
	}
	_, err := s.db.Exec(`UPDATE issues SET status = ?, resolved_by = ?, updated_at = ? WHERE id = ?`, status, resolver, now(), id)
	return err
}

// DeleteIssue removes an issue and its comments.
func (s *Store) DeleteIssue(id int64) error {
	_, err := s.db.Exec(`DELETE FROM issues WHERE id = ?`, id)
	return err
}

// Comments lists an issue's comments, oldest first.
func (s *Store) Comments(issueID int64) ([]Comment, error) {
	rows, err := s.db.Query(`SELECT c.id, c.user_id, COALESCE(u.username, ''), COALESCE(u.avatar, ''), c.body, c.created_at
		FROM issue_comments c LEFT JOIN users u ON u.id = c.user_id WHERE c.issue_id = ? ORDER BY c.id`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.UserID, &c.UserName, &c.UserAvatar, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddComment adds a comment and bumps the issue's updated time.
func (s *Store) AddComment(issueID, userID int64, body string) (int64, error) {
	t := now()
	res, err := s.db.Exec(`INSERT INTO issue_comments (issue_id, user_id, body, created_at) VALUES (?, ?, ?, ?)`, issueID, userID, body, t)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	_, err = s.db.Exec(`UPDATE issues SET updated_at = ? WHERE id = ?`, t, issueID)
	return id, err
}

// OpenIssuesForTitle lists a title's open issues, newest first.
func (s *Store) OpenIssuesForTitle(typ string, tmdbID int) ([]Issue, error) {
	rows, err := s.db.Query(`SELECT `+issueCols+` FROM issues i LEFT JOIN users u ON u.id = i.user_id
		WHERE i.type = ? AND i.tmdb_id = ? AND i.status = 'open' ORDER BY i.id DESC`, typ, tmdbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Issue{}
	for rows.Next() {
		i, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
