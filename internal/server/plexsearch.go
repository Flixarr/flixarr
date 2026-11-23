package server

import (
	"net/http"
	"sort"
	"strings"
	"unicode"

	"github.com/Flixarr/flixarr/internal/store"
)

// plexSearch finds titles in Plex by name as you type, forgiving typos (brkng bad and brekaing bad both find
// Breaking Bad), so what you already have shows first in search (#251).
func (s *Server) plexSearch(w http.ResponseWriter, r *http.Request, _ *store.User) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	all, err := s.Store.PlexTitles()
	if err != nil || len([]rune(q)) < 2 {
		writeJSON(w, http.StatusOK, []store.PlexTitle{})
		return
	}
	writeJSON(w, http.StatusOK, matchTitles(q, all, 10))
}

// matchTitles ranks titles against what was typed and keeps the best n that match at all.
func matchTitles(q string, all []store.PlexTitle, n int) []store.PlexTitle {
	nq := normTitle(q)
	type hit struct {
		t     store.PlexTitle
		score int
	}
	var hits []hit
	for _, t := range all {
		if sc := titleScore(normTitle(t.Title), nq); sc > 0 {
			hits = append(hits, hit{t, sc})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return len(hits[i].t.Title) < len(hits[j].t.Title)
	})
	out := []store.PlexTitle{}
	for i := 0; i < len(hits) && i < n; i++ {
		out = append(out, hits[i].t)
	}
	return out
}

var accents = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ñ", "n", "ç", "c", "&", " and ", "'", "", "’", "")

var roman = map[string]string{"ii": "2", "iii": "3", "iv": "4", "v": "5", "vi": "6", "vii": "7", "viii": "8", "ix": "9", "x": "10"}

// normTitle: lower case, no accents, & = and, roman numerals as numbers, punctuation as spaces, no leading "the".
func normTitle(s string) string {
	s = accents.Replace(strings.ToLower(s))
	words := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for i, w := range words {
		if n, ok := roman[w]; ok {
			words[i] = n
		}
	}
	if len(words) > 1 && (words[0] == "the" || words[0] == "a" || words[0] == "an") {
		words = words[1:]
	}
	return strings.Join(words, " ")
}

// titleScore: 100 exact, 95 same without spaces, 70 starts with the words, 55 starts with the letters,
// 50 has every word, 45 or 40 starts like it with one or two typos (a swapped, wrong, extra or missing letter),
// 30 has the letters in order (a typo that drops letters), 0 no match.
func titleScore(name, q string) int {
	if name == "" || q == "" {
		return 0
	}
	if name == q {
		return 100
	}
	nc, qc := strings.ReplaceAll(name, " ", ""), strings.ReplaceAll(q, " ", "")
	switch {
	case nc == qc:
		return 95
	case strings.HasPrefix(name, q+" "):
		return 70
	case strings.HasPrefix(nc, qc):
		return 55
	}
	words := " " + name + " "
	all := true
	for _, w := range strings.Fields(q) {
		if !strings.Contains(words, " "+w+" ") {
			all = false
			break
		}
	}
	if all {
		return 50
	}
	if len(qc) >= 4 && nc[0] == qc[0] {
		if d := typos(nc, qc); d <= len(qc)/4 {
			return 50 - 5*d
		}
		if inOrder(nc, qc) {
			return 30
		}
	}
	return 0
}

// typos is how many typos apart q is from the start of s: the fewest swapped neighbours, wrong, extra or
// missing letters (optimal string alignment distance against s's first len(q)-1 to len(q)+1 letters).
func typos(s, q string) int {
	a, b := []rune(q), []rune(s)
	if len(b) > len(a)+1 {
		b = b[:len(a)+1]
	}
	// d[i][j]: typos between a[:i] and b[:j]
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	// Typing a title is a prefix of it: the best match against s cut anywhere from len(q)-1 to len(q)+1.
	best := len(a)
	for j := max(0, len(a)-1); j <= len(b); j++ {
		best = min(best, d[len(a)][j])
	}
	return best
}

// inOrder reports whether every letter of q appears in s, in order.
func inOrder(s, q string) bool {
	i := 0
	for j := 0; j < len(s) && i < len(q); j++ {
		if s[j] == q[i] {
			i++
		}
	}
	return i == len(q)
}
