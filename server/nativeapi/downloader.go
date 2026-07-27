package nativeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
)

// addDownloaderRoute serves the navidrome-dl "Discover" API under /api/dl,
// bridging the frontend to a running `somedl web`. Only mounted when
// DL.SomedlURL is set; sits inside the authenticated group.
func (api *Router) addDownloaderRoute(r chi.Router) {
	if conf.Server.DL.SomedlURL == "" {
		return
	}
	sc := newSomedlClient(conf.Server.DL.SomedlURL)
	lib := libraryIndex{ds: api.ds}
	tracker := newDLTracker(sc)
	go tracker.run(context.Background())
	r.Route("/dl", func(r chi.Router) {
		r.Get("/search", dlSearch(sc, lib))
		r.Get("/album/{id}", dlAlbum(sc, lib))
		r.Get("/artist/{id}", dlArtist(sc, lib))
		r.Post("/download", dlDownload(sc, tracker))
		r.Post("/import", dlImport(sc, lib, api.playlists, tracker))
		r.Post("/complete/preview", dlCompletePreview(sc))
		r.Post("/complete", dlComplete(sc, tracker))
		r.Get("/status", dlStatus(sc, tracker))
		r.Post("/retry", dlRetry(tracker))
	})
}

func dlSearch(sc *somedlClient, lib libraryIndex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		typ := r.URL.Query().Get("type")
		var filter string
		switch typ {
		case "track":
			filter = "songs"
		case "artist":
			filter = "artists"
		default:
			typ, filter = "album", "albums"
		}
		result, err := sc.search(r.Context(), r.URL.Query().Get("q"), filter)
		if err != nil {
			dlError(w, r, err)
			return
		}
		cards := normalizeSearch(typ, result)
		lib.mark(r.Context(), cards)
		writeJSON(w, cards)
	}
}

func dlAlbum(sc *somedlClient, lib libraryIndex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		result, err := sc.albumByBrowseID(r.Context(), id)
		if err != nil {
			dlError(w, r, err)
			return
		}
		var listing struct {
			Title  string          `json:"title"`
			Tracks json.RawMessage `json:"tracks"`
		}
		_ = json.Unmarshal(result, &listing)
		tracks := normalizeTracks(listing.Tracks)
		lib.mark(r.Context(), tracks)
		writeJSON(w, map[string]any{"id": id, "title": listing.Title, "tracks": tracks})
	}
}

func dlArtist(sc *somedlClient, lib libraryIndex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		result, err := sc.artist(r.Context(), id)
		if err != nil {
			dlError(w, r, err)
			return
		}
		albums := normalizeSearch("album", section(result, "albums"))
		singles := normalizeSearch("album", section(result, "singles"))
		lib.mark(r.Context(), albums)
		lib.mark(r.Context(), singles)
		writeJSON(w, map[string]any{
			"id":      id,
			"name":    resultString(result, "name"),
			"albums":  albums,
			"singles": singles,
		})
	}
}

type downloadItem struct {
	URL     string `json:"url"`
	VideoID string `json:"videoId"`
	Query   string `json:"query"`
}

// precedence: url > videoId > query
func (it downloadItem) toItem() string {
	switch {
	case it.URL != "":
		return it.URL
	case it.VideoID != "":
		return videoURL(it.VideoID)
	default:
		return it.Query
	}
}

func videoURL(id string) string { return "https://music.youtube.com/watch?v=" + id }

func dlDownload(sc *somedlClient, tracker *dlTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Items []downloadItem `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			dlError(w, r, err)
			return
		}
		var items []string
		for _, it := range req.Items {
			if s := it.toItem(); s != "" {
				items = append(items, s)
			}
		}
		if len(items) == 0 {
			dlError(w, r, fmt.Errorf("no downloadable items"))
			return
		}
		resp, err := sc.enqueue(r.Context(), items)
		if err != nil {
			dlError(w, r, err)
			return
		}
		tracker.record(r.Context(), resp)
		writeJSON(w, map[string]any{"ok": true, "enqueued": len(items), "somedl": resp})
	}
}

func presentSet(nums []int) map[int]bool {
	m := make(map[int]bool, len(nums))
	for _, n := range nums {
		m[n] = true
	}
	return m
}

// albumMissing fetches a SomeDL album and returns its title, track count, and
// the tracks whose number isn't present locally.
func albumMissing(ctx context.Context, sc *somedlClient, albumID string, present map[int]bool) (title string, total int, missing []card, err error) {
	listing, err := sc.albumByBrowseID(ctx, albumID)
	if err != nil {
		return "", 0, nil, err
	}
	var l struct {
		Title  string          `json:"title"`
		Tracks json.RawMessage `json:"tracks"`
	}
	_ = json.Unmarshal(listing, &l)
	tracks := normalizeTracks(l.Tracks)
	return l.Title, len(tracks), missingTracks(present, tracks), nil
}

// dlCompletePreview resolves an album to fill (by SomeDL search or explicit
// albumId) and reports what's missing, without downloading anything. When
// searching it also returns the other candidates so the user can correct a
// wrong match before committing.
func dlCompletePreview(sc *somedlClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Artist  string `json:"artist"`
			Album   string `json:"album"`
			AlbumID string `json:"albumId"`
			Present []int  `json:"present"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			dlError(w, r, err)
			return
		}
		present := presentSet(req.Present)

		match := card{Kind: "album"}
		alternatives := []card{}
		if req.AlbumID != "" {
			match.ID = req.AlbumID
		} else {
			query := req.Artist + " " + req.Album
			result, err := sc.search(r.Context(), query, "albums")
			if err != nil {
				dlError(w, r, err)
				return
			}
			matches := normalizeSearch("album", result)
			if len(matches) == 0 {
				dlError(w, r, fmt.Errorf("no SomeDL album match for %q", query))
				return
			}
			match, alternatives = matches[0], matches[1:]
		}

		title, total, missing, err := albumMissing(r.Context(), sc, match.ID, present)
		if err != nil {
			dlError(w, r, err)
			return
		}
		if match.Title == "" {
			match.Title = title
		}
		writeJSON(w, map[string]any{
			"match": map[string]any{
				"id":          match.ID,
				"title":       match.Title,
				"artists":     match.Artists,
				"year":        match.Year,
				"thumbnail":   match.Thumbnail,
				"totalTracks": total,
				"missing":     missing,
			},
			"alternatives": alternatives,
		})
	}
}

// dlComplete downloads the missing tracks of a user-confirmed album (albumId
// comes from a prior preview, never a fresh best-guess).
func dlComplete(sc *somedlClient, tracker *dlTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AlbumID string `json:"albumId"`
			Present []int  `json:"present"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			dlError(w, r, err)
			return
		}
		if req.AlbumID == "" {
			dlError(w, r, fmt.Errorf("albumId required"))
			return
		}
		title, _, missing, err := albumMissing(r.Context(), sc, req.AlbumID, presentSet(req.Present))
		if err != nil {
			dlError(w, r, err)
			return
		}
		if len(missing) == 0 {
			writeJSON(w, map[string]any{"matched": title, "missing": 0})
			return
		}
		items := make([]string, len(missing))
		for i, c := range missing {
			items[i] = videoURL(c.ID)
		}
		resp, err := sc.enqueue(r.Context(), items)
		if err != nil {
			dlError(w, r, err)
			return
		}
		tracker.record(r.Context(), resp)
		writeJSON(w, map[string]any{"matched": title, "missing": len(missing), "tracks": missing})
	}
}

func dlStatus(sc *somedlClient, tracker *dlTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := sc.status(r.Context())
		if err != nil {
			dlError(w, r, err)
			return
		}
		writeJSON(w, normalizeStatus(resp, tracker))
	}
}

// dlRetry restarts downloads the retry budget ran out on, all of them when ids
// is empty.
func dlRetry(tracker *dlTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			dlError(w, r, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "restarted": tracker.retryNow(req.IDs)})
	}
}

// ytmusicapi nests artist lists as { key: { results: [...] } }.
func section(result json.RawMessage, key string) json.RawMessage {
	var obj map[string]struct {
		Results json.RawMessage `json:"results"`
	}
	_ = json.Unmarshal(result, &obj)
	return obj[key].Results
}

func resultString(result json.RawMessage, key string) string {
	var obj map[string]any
	_ = json.Unmarshal(result, &obj)
	s, _ := obj[key].(string)
	return s
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func dlError(w http.ResponseWriter, r *http.Request, err error) {
	log.Error(r, "navidrome-dl request failed", err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
