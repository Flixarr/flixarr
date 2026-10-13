package store

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// Permission switches. Roles are presets that set them; changing any switch makes the role Custom.
var AllPerms = []string{"admin", "manageRequests", "autoApprove", "requestMovies", "requestTv", "advanced", "viewRequests", "reportIssues", "manageIssues", "watchlist", "redownload"}

// RolePerms are the presets, in the order Users shows them.
var RolePerms = map[string][]string{
	"Admin":   AllPerms,
	"Trusted": {"autoApprove", "requestMovies", "requestTv", "advanced", "viewRequests", "reportIssues", "watchlist"},
	"User":    {"requestMovies", "requestTv", "reportIssues", "watchlist"},
}

// Can reports whether the user has a permission. Admins can do everything.
func (u *User) Can(perm string) bool {
	return slices.Contains(u.Perms, "admin") || slices.Contains(u.Perms, perm)
}

// RoleOf names a permission set: a preset when it matches one exactly, otherwise Custom.
func RoleOf(perms []string) string {
	if slices.Contains(perms, "admin") {
		return "Admin"
	}
	for _, r := range []string{"Trusted", "User"} {
		if sameSet(perms, RolePerms[r]) {
			return r
		}
	}
	return "Custom"
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !slices.Contains(b, x) {
			return false
		}
	}
	return true
}

// cleanPerms keeps known keys, once each, in AllPerms order.
func cleanPerms(in []string) []string {
	out := []string{}
	for _, p := range AllPerms {
		if slices.Contains(in, p) {
			out = append(out, p)
		}
	}
	return out
}

func splitPerms(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}

// ErrOwner is returned when a change would lock the owner out.
var ErrOwner = errors.New("the owner always stays an admin")

// Users lists everyone who can sign in, owner first, with how many requests each has made.
func (s *Store) Users() ([]UserRow, error) {
	rows, err := s.db.Query(`SELECT ` + userCols + `, (SELECT COUNT(*) FROM requests r WHERE r.user_id = users.id) FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserRow{}
	for rows.Next() {
		var row UserRow
		var perms string
		u := &row.User
		if err := rows.Scan(&u.ID, &u.PlexID, &u.Username, &u.Email, &u.Avatar, &u.Role, &perms, &u.CreatedAt, &u.Owner, &u.MovieLimit, &u.SeasonLimit, &row.Requests); err != nil {
			return nil, err
		}
		u.Perms = splitPerms(perms)
		out = append(out, row)
	}
	return out, rows.Err()
}

// UserRow is a user on the Users page.
type UserRow struct {
	User
	Requests int `json:"requests"`
}

// UserByID finds a user.
func (s *Store) UserByID(id int64) (*User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

// SetPerms changes a user's permission switches (and so their role label).
func (s *Store) SetPerms(id int64, perms []string) (*User, error) {
	u, err := s.UserByID(id)
	if err != nil {
		return nil, err
	}
	perms = cleanPerms(perms)
	if u.Owner && !slices.Contains(perms, "admin") {
		return nil, ErrOwner
	}
	if _, err := s.db.Exec(`UPDATE users SET perms = ?, role = ? WHERE id = ?`, strings.Join(perms, ","), RoleOf(perms), id); err != nil {
		return nil, err
	}
	return s.UserByID(id)
}

// DeleteUser removes a user, their sessions and their requests (nothing changes in Radarr/Sonarr).
func (s *Store) DeleteUser(id int64) error {
	u, err := s.UserByID(id)
	if err != nil {
		return err
	}
	if u.Owner {
		return ErrOwner
	}
	_, err = s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

// SetLimits changes a user's request limits (per 7 days, 0 = unlimited).
func (s *Store) SetLimits(id int64, movies, seasons int) error {
	_, err := s.db.Exec(`UPDATE users SET movie_limit = ?, season_limit = ? WHERE id = ?`, max(movies, 0), max(seasons, 0), id)
	return err
}

// RecentUsage is what a user requested in the last 7 days: movies, and seasons of series.
// Declined and failed requests don't count.
func (s *Store) RecentUsage(userID int64) (movies, seasons int, err error) {
	err = s.db.QueryRow(`SELECT COALESCE(SUM(type = 'movie'), 0), COALESCE(SUM(CASE WHEN type = 'series' THEN MAX(season_count, 1) ELSE 0 END), 0)
		FROM requests WHERE user_id = ? AND status NOT IN ('declined', 'failed') AND created_at > ?`, userID, time.Now().Add(-7*24*time.Hour).UTC().Format(time.RFC3339)).Scan(&movies, &seasons)
	return
}
