package nativeapi

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"github.com/navidrome/navidrome/model"
)

// libraryIndex flags Discover cards that already exist in the local library,
// matched by normalized name (+ artist), so the UI can badge/disable them. It
// uses the same full-text Search the UI does, then confirms with a normalized
// equality check to reject loose FTS hits.
type libraryIndex struct{ ds model.DataStore }

const libMatchMax = 15

var parenRe = regexp.MustCompile(`[(\[][^)\]]*[)\]]`)

// normKey lowercases, drops parenthetical/bracketed segments (e.g.
// "(Remastered 2011)", "[Deluxe]") and reduces to space-separated
// letters/digits, so editions and punctuation don't defeat matching.
func normKey(s string) string {
	s = parenRe.ReplaceAllString(strings.ToLower(s), " ")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func (l libraryIndex) mark(ctx context.Context, cards []card) {
	for i := range cards {
		cards[i].InLibrary = l.has(ctx, cards[i])
	}
}

func (l libraryIndex) has(ctx context.Context, c card) bool {
	if normKey(c.Title) == "" {
		return false
	}
	switch c.Kind {
	case "album":
		return l.hasAlbum(ctx, c)
	case "artist":
		return l.hasArtist(ctx, c)
	case "track":
		return l.hasTrack(ctx, c)
	}
	return false
}

func (l libraryIndex) hasAlbum(ctx context.Context, c card) bool {
	albums, err := l.ds.Album(ctx).Search(searchQuery(c), model.QueryOptions{Max: libMatchMax})
	if err != nil {
		return false
	}
	want := normKey(c.Title)
	for _, a := range albums {
		if !a.Missing && normKey(a.Name) == want && artistMatch(c.Artists, a.AlbumArtist) {
			return true
		}
	}
	return false
}

func (l libraryIndex) hasArtist(ctx context.Context, c card) bool {
	artists, err := l.ds.Artist(ctx).Search(c.Title, model.QueryOptions{Max: libMatchMax})
	if err != nil {
		return false
	}
	want := normKey(c.Title)
	for _, a := range artists {
		if normKey(a.Name) == want {
			return true
		}
	}
	return false
}

func (l libraryIndex) hasTrack(ctx context.Context, c card) bool {
	_, ok := l.trackID(ctx, c)
	return ok
}

// trackID returns the id of the library MediaFile matching a track card, if any.
func (l libraryIndex) trackID(ctx context.Context, c card) (string, bool) {
	if normKey(c.Title) == "" {
		return "", false
	}
	mfs, err := l.ds.MediaFile(ctx).Search(searchQuery(c), model.QueryOptions{Max: libMatchMax})
	if err != nil {
		return "", false
	}
	want := normKey(c.Title)
	for _, m := range mfs {
		if !m.Missing && normKey(m.Title) == want && artistMatch(c.Artists, m.Artist) {
			return m.ID, true
		}
	}
	return "", false
}

func searchQuery(c card) string {
	if len(c.Artists) > 0 {
		return c.Title + " " + c.Artists[0]
	}
	return c.Title
}

// artistMatch is permissive: no card artist (or unknown library artist) doesn't
// block a name match; otherwise a card artist must appear in the library value.
func artistMatch(cardArtists []string, libArtist string) bool {
	if len(cardArtists) == 0 {
		return true
	}
	lib := normKey(libArtist)
	if lib == "" {
		return true
	}
	for _, a := range cardArtists {
		if na := normKey(a); na != "" && strings.Contains(lib, na) {
			return true
		}
	}
	return false
}
